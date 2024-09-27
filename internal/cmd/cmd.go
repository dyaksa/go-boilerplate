package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/telkomindonesia/go-boilerplate/internal/httpserver"
	"github.com/telkomindonesia/go-boilerplate/internal/kafka"
	"github.com/telkomindonesia/go-boilerplate/internal/postgres"
	"github.com/telkomindonesia/go-boilerplate/internal/tenantservice"
	"github.com/telkomindonesia/go-boilerplate/pkg/cmd"
	"github.com/telkomindonesia/go-boilerplate/pkg/httpclient"
	"github.com/telkomindonesia/go-boilerplate/pkg/log"
	"github.com/telkomindonesia/go-boilerplate/pkg/log/types"
	"github.com/telkomindonesia/go-boilerplate/pkg/tinkx"
	"github.com/telkomindonesia/go-boilerplate/pkg/tlswrap"
	"github.com/telkomindonesia/go-boilerplate/pkg/util"
)

type OptFunc func(*CMD) error

func WithEnvPrefix(p string) OptFunc {
	return func(s *CMD) (err error) {
		s.envPrefix = p
		return
	}
}

func WithoutDotEnv() OptFunc {
	return func(s *CMD) (err error) {
		s.dotenv = false
		return
	}
}

func WithCanceler(f func(context.Context) context.Context) OptFunc {
	return func(s *CMD) (err error) {
		s.canceler = f
		return
	}
}

func WithOtelLoader(f func(ctx context.Context) func()) OptFunc {
	return func(s *CMD) (err error) {
		s.otelLoader = f
		return
	}
}

var _ log.Loggable = CMD{}

type CMD struct {
	envPrefix string
	dotenv    bool

	HTTPAddr             string                    `env:"HTTP_LISTEN_ADDRESS,expand" envDefault:":8080" json:"http_listen_addr"`
	PostgresUrl          types.MaskedStringUserURL `env:"POSTGRES_URL,required,notEmpty,expand" json:"postgres_url"`
	KafkaBrokers         []string                  `env:"KAFKA_BROKERS,expand" json:"kafka_brokers"`
	KafkaTopicOutbox     string                    `env:"KAFKA_TOPIC_OUTBOX,expand" json:"kafka_topic_outbox"`
	TenantServiceBaseUrl types.MaskedStringUserURL `env:"TENANT_SERVICE_BASE_URL,required,notEmpty,expand" json:"tenant_service_base_url"`

	CMD        *cmd.CMD `env:"-" json:"cmd"`
	logger     log.Logger
	aead       *tinkx.DerivableKeyset[tinkx.PrimitiveAEAD]
	bidx       *tinkx.DerivableKeyset[tinkx.PrimitiveBIDX]
	hc         httpclient.HTTPClient
	tlsw       *tlswrap.TLSWrap
	canceler   func(ctx context.Context) context.Context
	otelLoader func(ctx context.Context) func()

	h  *httpserver.HTTPServer
	p  *postgres.Postgres
	k  *kafka.Kafka
	ts *tenantservice.TenantService

	closers []func(context.Context) error
}

func New(opts ...OptFunc) (c *CMD, err error) {
	c = &CMD{
		envPrefix: "PROFILE_",
		dotenv:    true,
	}
	for _, opt := range opts {
		if err = opt(c); err != nil {
			return
		}
	}
	err = util.LoadEnv(c, util.LoadEnvOptions{
		Prefix: c.envPrefix,
		DotEnv: c.dotenv,
	})
	if err != nil {
		return nil, err
	}

	if err = c.initCMD(); err != nil {
		return
	}
	if err = c.initKafka(); err != nil {
		return
	}
	if err = c.initPostgres(); err != nil {
		return
	}
	if err = c.initTenantService(); err != nil {
		return
	}
	if err = c.initHTTPServer(); err != nil {
		return
	}

	return
}

func (c *CMD) initCMD() (err error) {
	c.CMD, err = cmd.New(cmd.WithEnv(c.envPrefix, c.dotenv))
	if err != nil {
		return fmt.Errorf("failed to instantiate cmd: %w", err)
	}

	if c.otelLoader == nil {
		c.otelLoader = c.CMD.LoadOtel
	}
	if c.canceler == nil {
		c.canceler = c.CMD.CancelOnExit
	}
	c.logger = util.Require(c.CMD.Logger, log.Global().WithLog(log.String("name", "logger")))
	c.aead = util.Require(c.CMD.AEADDerivableKeyset, c.logger.WithLog(log.String("name", "aead")))
	c.bidx = util.Require(c.CMD.BIDXDerivableKeyset, c.logger.WithLog(log.String("name", "blind-idx")))
	c.tlsw = util.Require(c.CMD.TLSWrap, c.logger.WithLog(log.String("name", "tlswrapper")))
	c.hc = util.Require(c.CMD.HTTPClient, c.logger.WithLog(log.String("name", "httpclient")))
	return
}

func (c *CMD) initKafka() (err error) {
	if len(c.KafkaBrokers) == 0 {
		return
	}

	c.k, err = kafka.New(
		kafka.WithBrokers(c.KafkaBrokers),
	)
	if err != nil {
		return fmt.Errorf("failed to instantiate kafka: %w", err)
	}

	if c.k != nil && c.KafkaTopicOutbox == "" {
		return fmt.Errorf("invalid kafka outboox topic: %s", c.KafkaTopicOutbox)
	}

	c.closers = append(c.closers, c.k.Close)
	return
}

func (c *CMD) initPostgres() (err error) {
	opts := []postgres.OptFunc{
		postgres.WithConnString(c.PostgresUrl.String()),
		postgres.WithDerivableKeysets(c.aead, c.bidx),
		postgres.WithLogger(c.logger.WithLog(log.String("logger-name", "postgres"))),
	}
	if c.k != nil {
		opts = append(opts, postgres.WithOutboxCERelayFunc(c.k.OutboxCERelayFunc()))
	}
	c.p, err = postgres.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to instantiate postges: %w", err)
	}

	c.closers = append(c.closers, c.p.Close)
	return
}

func (c *CMD) initTenantService() (err error) {
	c.ts, err = tenantservice.New(
		tenantservice.WithBaseUrl(c.TenantServiceBaseUrl.String()),
		tenantservice.WithHTTPClient(c.hc.Client),
		tenantservice.WithLogger(c.logger.WithLog(log.String("logger-name", "tenant-service"))),
	)
	if err != nil {
		return fmt.Errorf("failed to instantiate tenant service: %w", err)
	}
	return
}

func (c *CMD) initHTTPServer() (err error) {
	l, err := net.Listen("tcp", c.HTTPAddr)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	c.h, err = httpserver.New(
		httpserver.WithListener(c.tlsw.Listener(l)),
		httpserver.WithProfileRepository(c.p),
		httpserver.WithTenantRepository(c.ts),
		httpserver.WithLogger(c.logger.WithLog(log.String("logger-name", "http-server"))),
	)
	if err != nil {
		return fmt.Errorf("failed to instantiate http server: %w", err)
	}

	c.closers = append(c.closers, c.h.Close)
	return
}

func (c *CMD) Run(ctx context.Context) (err error) {
	defer func() { c.logger.Error("error", log.Error("error", err)) }()
	defer func() { err = c.close(ctx, err) }()
	defer c.otelLoader(ctx)()

	c.logger.Info("server starting", log.Any("server", c))
	return c.h.Start(c.canceler(ctx))
}

func (c *CMD) close(ctx context.Context, err error) error {
	for _, fn := range c.closers {
		err = errors.Join(err, fn(ctx))
	}
	return err
}

func (c CMD) AsLog() any {
	return types.AsLog(c)
}
