package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 为 ORSync 运行所需的全部配置项（无隐式默认值，缺失必填项时 Load 返回错误）。
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Outline  OutlineConfig  `mapstructure:"outline"`
	RAGFlow  RAGFlowConfig  `mapstructure:"ragflow"`
	Webhook  WebhookConfig  `mapstructure:"webhook"`
}

type ServerConfig struct {
	Addr string `mapstructure:"addr"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type OutlineConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
}

type RAGFlowConfig struct {
	BaseURL   string `mapstructure:"base_url"`
	APIKey    string `mapstructure:"api_key"`
	DatasetID string `mapstructure:"dataset_id"`
}

type WebhookConfig struct {
	// Secret 为空表示不校验 Outline Webhook 签名/密钥（Phase 4 再接入校验逻辑）。
	Secret string `mapstructure:"secret"`
}

// Load 从配置文件与环境变量合并加载配置。
//
// 配置文件查找顺序（configFile 为命令行传入路径，可为空）：
//  1. 若 configFile 非空：读取该路径（须存在且可读）。
//  2. 否则若环境变量 ORSYNC_CONFIG_FILE 非空：读取该路径。
//  3. 否则：在当前工作目录下查找 config.yaml（可不存在，此时仅依赖环境变量等来源）。
//
// 环境变量使用大写+下划线，例如 SERVER_ADDR、OUTLINE_API_KEY。
func Load(configFile string) (Config, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnv := func(configKey, envVar string) {
		_ = v.BindEnv(configKey, envVar)
	}
	bindEnv("server.addr", "SERVER_ADDR")
	bindEnv("database.path", "DATABASE_PATH")
	bindEnv("outline.base_url", "OUTLINE_BASE_URL")
	bindEnv("outline.api_key", "OUTLINE_API_KEY")
	bindEnv("ragflow.base_url", "RAGFLOW_BASE_URL")
	bindEnv("ragflow.api_key", "RAGFLOW_API_KEY")
	bindEnv("ragflow.dataset_id", "RAGFLOW_DATASET_ID")
	bindEnv("webhook.secret", "WEBHOOK_SECRET")

	configFile = strings.TrimSpace(configFile)
	if configFile == "" {
		configFile = strings.TrimSpace(os.Getenv("ORSYNC_CONFIG_FILE"))
	}

	if configFile != "" {
		v.SetConfigFile(configFile)
		switch strings.ToLower(filepath.Ext(configFile)) {
		case ".yaml", ".yml", "":
			v.SetConfigType("yaml")
		default:
			v.SetConfigType("yaml")
		}
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config file %q: %w", configFile, err)
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return Config{}, fmt.Errorf("read config.yaml in working directory: %w", err)
			}
		}
	}

	if _, err := os.Stat(".env"); err == nil {
		v.SetConfigFile(".env")
		v.SetConfigType("env")
		if err := v.MergeInConfig(); err != nil {
			return Config{}, fmt.Errorf("merge .env: %w", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("stat .env: %w", err)
	}

	// .env 常用扁平大写键，补齐到嵌套路径以便 Unmarshal。
	liftFlatKeys(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	var missing []string
	if strings.TrimSpace(c.Server.Addr) == "" {
		missing = append(missing, "server.addr")
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		missing = append(missing, "database.path")
	}
	if strings.TrimSpace(c.Outline.BaseURL) == "" {
		missing = append(missing, "outline.base_url")
	}
	if strings.TrimSpace(c.Outline.APIKey) == "" {
		missing = append(missing, "outline.api_key")
	}
	if strings.TrimSpace(c.RAGFlow.BaseURL) == "" {
		missing = append(missing, "ragflow.base_url")
	}
	if strings.TrimSpace(c.RAGFlow.APIKey) == "" {
		missing = append(missing, "ragflow.api_key")
	}
	if strings.TrimSpace(c.RAGFlow.DatasetID) == "" {
		missing = append(missing, "ragflow.dataset_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

// liftFlatKeys 在嵌套键为空时，用大写扁平键填充（兼容 config.yaml 嵌套 + .env 扁平混用）。
func liftFlatKeys(v *viper.Viper) {
	lifts := []struct {
		nested, flat string
	}{
		{"server.addr", "SERVER_ADDR"},
		{"database.path", "DATABASE_PATH"},
		{"outline.base_url", "OUTLINE_BASE_URL"},
		{"outline.api_key", "OUTLINE_API_KEY"},
		{"ragflow.base_url", "RAGFLOW_BASE_URL"},
		{"ragflow.api_key", "RAGFLOW_API_KEY"},
		{"ragflow.dataset_id", "RAGFLOW_DATASET_ID"},
		{"webhook.secret", "WEBHOOK_SECRET"},
	}
	for _, p := range lifts {
		if strings.TrimSpace(v.GetString(p.nested)) != "" {
			continue
		}
		if s := strings.TrimSpace(v.GetString(p.flat)); s != "" {
			v.Set(p.nested, s)
		}
	}
}
