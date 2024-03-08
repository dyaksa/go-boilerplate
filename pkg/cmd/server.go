package cmd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/telkomindonesia/go-boilerplate/pkg/cert"
	"github.com/telkomindonesia/go-boilerplate/pkg/httpserver"
	"github.com/telkomindonesia/go-boilerplate/pkg/logger"
	"github.com/telkomindonesia/go-boilerplate/pkg/logger/zap"
	"github.com/telkomindonesia/go-boilerplate/pkg/postgres"
	"github.com/telkomindonesia/go-boilerplate/pkg/tenantservice"
	"github.com/telkomindonesia/go-boilerplate/pkg/util"
)

type ServerOptFunc func(*Server) error

func ServerWithEnvPrefix(p string) ServerOptFunc {
	return func(s *Server) (err error) {
		s.envPrefix = p
		return
	}
}

func ServerWithOutDotEnv(p string) ServerOptFunc {
	return func(s *Server) (err error) {
		s.dotenv = false
		return
	}
}

type Server struct {
	envPrefix string
	dotenv    bool

	HTTPAddr     string  `env:"HTTP_LISTEN_ADDRESS,expand"`
	HTTPKeyPath  *string `env:"HTTP_TLS_KEY_PATH"`
	HTTPCertPath *string `env:"HTTP_TLS_CERT_PATH"`
	HTTPCA       *string `env:"HTTP_CA_CERTS_PATHS"`

	PostgresUrl      string `env:"POSTGRES_URL,required,notEmpty,expand"`
	PostgresAEADPath string `env:"POSTGRES_AEAD_KEY_PATH,required,notEmpty"`
	PostgresMACPath  string `env:"POSTGRES_MAC_KEY_PATH,required,notEmpty"`

	TenantServiceBaseUrl string `env:"TENANT_SERVICE_BASE_URL,required,notEmpty,expand"`

	l  logger.Logger
	h  *httpserver.HTTPServer
	p  *postgres.Postgres
	ts *tenantservice.TenantService
	hc *http.Client

	closeable []func(context.Context) error
}

func NewServer(opts ...ServerOptFunc) (s *Server, err error) {
	s = &Server{envPrefix: "PROFILE_", dotenv: true}
	for _, opt := range opts {
		if err = opt(s); err != nil {
			return
		}
	}

	err = util.LoadFromEnv(s, util.LoadEnvOptions{
		Prefix: s.envPrefix,
		DotEnv: s.dotenv,
	})
	if err != nil {
		return nil, err
	}

	if err = s.initLogger(); err != nil {
		return
	}
	if err = s.initPostgres(); err != nil {
		return
	}
	if err = s.initHTTPClient(); err != nil {
		return
	}
	if err = s.initTenantService(); err != nil {
		return
	}
	if err = s.initHTTPServer(); err != nil {
		return
	}

	return
}

func (s *Server) initLogger() (err error) {
	s.l, err = zap.New()
	if err != nil {
		return fmt.Errorf("fail to instantiate logger: %w", err)
	}
	return
}

func (s *Server) initPostgres() (err error) {
	s.p, err = postgres.New(
		postgres.WithConnString(s.PostgresUrl),
		postgres.WithInsecureKeysetFiles(s.PostgresAEADPath, s.PostgresMACPath),
		postgres.WithLogger(s.l),
	)
	if err != nil {
		return fmt.Errorf("fail to instantiate postges: %w", err)
	}
	s.closeable = append(s.closeable, s.p.Close)
	return
}

func (s *Server) initHTTPClient() (err error) {
	var cfg *tls.Config
	if s.HTTPCA != nil {
		cfg = &tls.Config{}
		cw, err := cert.NewCAWatcher(cfg, *s.HTTPCA, true, s.l)
		if err != nil {
			return fmt.Errorf("fail to init ca:%w", err)
		}
		s.closeable = append(s.closeable, cw.Close)
	}

	s.hc = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).Dial,
			TLSClientConfig:     cfg,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
	return
}

func (s *Server) initTenantService() (err error) {
	s.ts, err = tenantservice.New(
		tenantservice.WithBaseUrl(s.TenantServiceBaseUrl),
		tenantservice.WithHTTPClient(s.hc),
		tenantservice.WithLogger(s.l),
	)
	if err != nil {
		return fmt.Errorf("fail to instantiate tenant service:%w", err)
	}
	return
}

func (s *Server) initHTTPServer() (err error) {
	opts := []httpserver.OptFunc{
		httpserver.WithAddr(s.HTTPAddr),
		httpserver.WithProfileRepository(s.p),
		httpserver.WithTenantRepository(s.ts),
		httpserver.WithLogger(s.l),
	}
	if s.HTTPKeyPath != nil && s.HTTPCertPath != nil {
		opts = append(opts, httpserver.WithTLS(*s.HTTPKeyPath, *s.HTTPCertPath))
	}

	s.h, err = httpserver.New(opts...)
	if err != nil {
		return fmt.Errorf("fail to instantiate http server: %w", err)
	}
	s.closeable = append(s.closeable, s.h.Close)
	return
}

func (s *Server) Run(ctx context.Context) (err error) {
	err = s.h.Start(ctx)
	defer func() {
		for _, fn := range s.closeable {
			err = errors.Join(err, fn(ctx))
		}
	}()
	return
}
