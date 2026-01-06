package public

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/eryajf/cloud_dns_exporter/public/logger"
	"github.com/rs/xid"

	"gopkg.in/yaml.v2"
)

// InitSvc 初始化服务
func InitSvc() {
	LoadConfig()
	InitCache()
}

const (
	// Custom
	CustomRecords string = "custom_records"
	// Cloud Providers
	TencentDnsProvider    string = "tencent"
	AliyunDnsProvider     string = "aliyun"
	GodaddyDnsProvider    string = "godaddy"
	DNSLaDnsProvider      string = "dnsla"
	AmazonDnsProvider     string = "amazon"
	CloudFlareDnsProvider string = "cloudflare"
	// Metrics Name
	DomainList     string = "domain_list"
	RecordList     string = "record_list"
	RecordCertInfo string = "record_cert_info"
	BuildInfo      string = "build_info"
	// Cron Config Defaults
	DefaultDomainRecordSyncInterval = 30   // 默认域名记录同步间隔（秒）
	DefaultCertInfoSyncInterval     = 3600 // 默认证书信息同步间隔（秒），24小时
)

var (
	once      sync.Once
	Config    *Configuration
	Cache     *bigcache.BigCache
	CertCache *bigcache.BigCache
)

type Account struct {
	CloudProvider string `yaml:"cloud_provider"`
	CloudName     string `yaml:"cloud_name"`
	SecretID      string `yaml:"secretId"`
	SecretKey     string `yaml:"secretKey"`
}

// CronConfig 定时任务配置
type CronConfig struct {
	DomainRecordSyncInterval int `yaml:"domain_record_sync_interval"` // 域名记录同步间隔（秒）
	CertInfoSyncInterval     int `yaml:"cert_info_sync_interval"`     // 证书信息同步间隔（秒）
}

// Config 表示配置文件的结构
type Configuration struct {
	CronConfig     *CronConfig `yaml:"cron_config"`
	CustomRecords  []string    `yaml:"custom_records"`
	CloudProviders map[string]struct {
		Accounts []map[string]string `yaml:"accounts"`
	} `yaml:"cloud_providers"`
}

// LoadConfig 加载配置
func LoadConfig() *Configuration {
	once.Do(func() {
		Config = &Configuration{}
		data, err := os.ReadFile("config.yaml")
		if err != nil {
			logger.Fatal("read config file failed: ", err)
		}
		err = yaml.Unmarshal(data, &Config)
		if err != nil {
			logger.Fatal("unmarshal config file failed: ", err)
		}
	})
	return Config
}

// InitCache 初始化缓存
func InitCache() {
	var err error
	Cache, err = bigcache.New(context.Background(), bigcache.DefaultConfig(5*time.Minute))
	if err != nil {
		logger.Fatal("init cache failed: ", err)
	}
	CertCache, err = bigcache.New(context.Background(), bigcache.DefaultConfig(25*time.Hour))
	if err != nil {
		logger.Fatal("init cache failed: ", err)
	}
}

// GetID 获取唯一ID
func GetID() string {
	return xid.New().String()
}

// GetCronConfig 获取定时任务配置，如果未配置则返回默认值
func GetCronConfig() CronConfig {
	if Config.CronConfig == nil {
		return CronConfig{
			DomainRecordSyncInterval: DefaultDomainRecordSyncInterval,
			CertInfoSyncInterval:     DefaultCertInfoSyncInterval,
		}
	}

	config := *Config.CronConfig
	// 如果配置了但值为0或负数，使用默认值
	if config.DomainRecordSyncInterval <= 0 {
		config.DomainRecordSyncInterval = DefaultDomainRecordSyncInterval
	}
	if config.CertInfoSyncInterval <= 0 {
		config.CertInfoSyncInterval = DefaultCertInfoSyncInterval
	}

	return config
}
