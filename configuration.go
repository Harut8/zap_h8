package zap_h8

import (
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
)

var (
	atomicLevel = zap.NewAtomicLevel()
	once        sync.Once
	logger      *zap.Logger
)

type LogRotationConfig struct {
	MaxSize    int  `yaml:"maxSize"`
	MaxBackups int  `yaml:"maxBackups"`
	MaxAge     int  `yaml:"maxAge"`
	Compress   bool `yaml:"compress"`
}

type Config struct {
	Level       string              `yaml:"level"`
	Development bool                `yaml:"development"`
	Encoding    string              `yaml:"encoding"`
	OutputPath  string              `yaml:"outputPath"`
	Sampling    *zap.SamplingConfig `yaml:"sampling"`
	Rotation    LogRotationConfig   `yaml:"rotation"`
}

func loadConfig(configPath string) (*Config, error) {
	yamlData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading log config: %w", err)
	}
	var cfg Config
	err = yaml.Unmarshal(yamlData, &cfg)
	if err != nil {
		return nil, fmt.Errorf("error parsing log config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) getEncoder() zapcore.Encoder {
	var encoderConfig zapcore.EncoderConfig
	if c.Development {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
	}
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	if c.Encoding == "json" {
		return zapcore.NewJSONEncoder(encoderConfig)
	}
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func (c *Config) setLogLevel() {
	parsedLevel, err := zapcore.ParseLevel(c.Level)
	if err != nil {
		parsedLevel = zap.InfoLevel
	}
	atomicLevel.SetLevel(parsedLevel)
}
func (c *Config) getWriter() zapcore.WriteSyncer {
	if c.Development {
		return zapcore.AddSync(os.Stdout)
	}
	return zapcore.AddSync(&lumberjack.Logger{
		Filename:   c.OutputPath,
		MaxSize:    c.Rotation.MaxSize,
		MaxBackups: c.Rotation.MaxBackups,
		MaxAge:     c.Rotation.MaxAge,
		Compress:   c.Rotation.Compress,
	})
}

func (c *Config) buildLogger() *zap.Logger {
	c.setLogLevel()
	core := zapcore.NewCore(
		c.getEncoder(),
		c.getWriter(),
		atomicLevel,
	)

	options := []zap.Option{
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	}

	if c.Sampling != nil {
		options = append(options, zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewSamplerWithOptions(core, time.Second, c.Sampling.Initial, c.Sampling.Thereafter)
		}))
	}

	return zap.New(core, options...)
}
func (c *Config) String() string {
	return fmt.Sprintf("Level: %s, Development: %t, OutputPath: %s", c.Level, c.Development, c.OutputPath)
}

func GetLogger(configPath string) (*zap.Logger, error) {
	var initErr error
	once.Do(func() {
		cfg, err := loadConfig(configPath)
		if err != nil {
			fmt.Printf("Error initializing logger: %v\n", err)
			initErr = err
			logger = zap.NewNop() // No-op logger to prevent crashes
			return
		}
		logger = cfg.buildLogger()
		if logger != nil {
			logger.Info("Logger initialized",
				zap.String("level", cfg.Level),
				zap.Bool("development", cfg.Development),
				zap.String("output_path", cfg.OutputPath),
			)
		} else {
			fmt.Println("Failed to create logger")
			return
		}
	})

	if initErr != nil {
		return nil, initErr
	}
	return logger, nil
}
