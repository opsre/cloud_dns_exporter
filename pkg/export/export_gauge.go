package export

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/eryajf/cloud_dns_exporter/public/logger"

	"github.com/eryajf/cloud_dns_exporter/pkg/provider"
	"github.com/eryajf/cloud_dns_exporter/public"
	"github.com/prometheus/client_golang/prometheus"
)

// 指标结构体
type Metrics struct {
	metrics   map[string]*prometheus.Desc
	mutex     sync.Mutex
	version   string
	gitCommit string
	goVersion string
	osArch    string
	buildTime string
}

// newGlobalMetric 创建指标描述符
func newGlobalMetric(namespace string, metricName string, docString string, labels []string) *prometheus.Desc {
	if namespace == "" {
		return prometheus.NewDesc(metricName, docString, labels, nil)
	} else {
		return prometheus.NewDesc(namespace+"_"+metricName, docString, labels, nil)
	}
}

// NewMetrics 初始化指标信息，即Metrics结构体
func NewMetrics(namespace, version, gitCommit, goVersion, osArch, buildTime string) *Metrics {
	return &Metrics{
		version:   version,
		gitCommit: gitCommit,
		goVersion: goVersion,
		osArch:    osArch,
		buildTime: buildTime,
		metrics: map[string]*prometheus.Desc{
			public.DomainList: newGlobalMetric(namespace,
				public.DomainList,
				"Cloud Domain List",
				[]string{
					"cloud_provider",
					"cloud_name",
					"domain_id",
					"domain_name",
					"domain_remark",
					"domain_status",
					"created_date",
					"expiry_date",
				}),
			public.RecordList: newGlobalMetric(namespace,
				public.RecordList,
				"Cloud Doamin Record List",
				[]string{
					"cloud_provider",
					"cloud_name",
					"domain_name",
					"record_id",
					"record_type",
					"record_name",
					"record_value",
					"record_ttl",
					"record_weight",
					"record_status",
					"record_remark",
					"update_time",
					"full_record",
				}),
			public.RecordCertInfo: newGlobalMetric(namespace,
				public.RecordCertInfo,
				"Cloud Doamin Record Cert Info",
				[]string{
					"cloud_provider",
					"cloud_name",
					"domain_name",
					"record_id",
					"full_record",
					"subject_common_name",
					"subject_organization",
					"subject_organizational_unit",
					"issuer_common_name",
					"issuer_organization",
					"issuer_organizational_unit",
					"created_date",
					"expiry_date",
					"cert_matched",
					"error_msg",
				}),
			public.BuildInfo: newGlobalMetric(namespace,
				public.BuildInfo,
				"Application build information",
				[]string{
					"version",
					"git_commit",
					"go_version",
					"os_arch",
					"build_time",
				}),
		},
	}
}

// Describe 传递结构体中的指标描述符到channel
func (c *Metrics) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m
	}
}

// Collect 抓取最新的数据，传递给channel
func (c *Metrics) Collect(ch chan<- prometheus.Metric) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for cloudProvider, accounts := range public.Config.CloudProviders {
		for _, cloudAccount := range accounts.Accounts {
			cloudName := cloudAccount["name"]
			// get domain list from cache
			domainListCacheKey := public.DomainList + "_" + cloudProvider + "_" + cloudName
			var domains []provider.Domain
			domainListCacheValue, err := public.Cache.Get(domainListCacheKey)
			if err != nil {
				logger.Error(fmt.Sprintf("[ %s ] get domain list failed: %v", domainListCacheKey, err))
				continue
			}
			err = json.Unmarshal(domainListCacheValue, &domains)
			if err != nil {
				logger.Error(fmt.Sprintf("[ %s ] json.Unmarshal error: %v", domainListCacheKey, err))
				continue
			}
			for _, v := range domains {
				ch <- prometheus.MustNewConstMetric(
					c.metrics[public.DomainList], prometheus.GaugeValue, float64(v.DaysUntilExpiry), v.CloudProvider, v.CloudName, v.DomainID, v.DomainName, v.DomainRemark, v.DomainStatus, v.CreatedDate, v.ExpiryDate)
			}
			// get record list from cache
			recordListCacheKey := public.RecordList + "_" + cloudProvider + "_" + cloudName
			var records []provider.Record
			recordListCacheValue, err := public.Cache.Get(recordListCacheKey)
			if err != nil {
				logger.Error(fmt.Sprintf("[ %s ] get record list failed: %v", domainListCacheKey, err))
				continue
			}
			err = json.Unmarshal(recordListCacheValue, &records)
			if err != nil {
				logger.Error(fmt.Sprintf("[ %s ] json.Unmarshal error: %v", domainListCacheKey, err))
				continue
			}
			for _, v := range records {
				if v.RecordName == "@" && v.RecordType == "NS" { // Special Record, Skip it.
					continue
				}
				ch <- prometheus.MustNewConstMetric(
					c.metrics[public.RecordList], prometheus.GaugeValue, 1, v.CloudProvider, v.CloudName, v.DomainName, v.RecordID, v.RecordType, v.RecordName, v.RecordValue, v.RecordTTL, v.RecordWeight, v.RecordStatus, v.RecordRemark, v.UpdateTime, v.FullRecord)
			}
			// get record cert info list from cache
			recordCertInfoCacheKey := public.RecordCertInfo + "_" + cloudProvider + "_" + cloudName
			var recordCerts []provider.RecordCert
			recordCertInfoCacheValue, err := public.CertCache.Get(recordCertInfoCacheKey)
			if err != nil {
				logger.Error(fmt.Sprintf("[ %s ] get record list failed: %v", recordCertInfoCacheKey, err))
				continue
			}
			err = json.Unmarshal(recordCertInfoCacheValue, &recordCerts)
			if err != nil {
				logger.Error(fmt.Sprintf("[ %s ] json.Unmarshal error: %v", recordCertInfoCacheKey, err))
				continue
			}
			for _, v := range recordCerts {
				if v.RecordID == "" {
					continue
				}
				ch <- prometheus.MustNewConstMetric(c.metrics[public.RecordCertInfo], prometheus.GaugeValue, float64(v.DaysUntilExpiry), v.CloudProvider, v.CloudName, v.DomainName, v.RecordID, v.FullRecord, v.SubjectCommonName, v.SubjectOrganization, v.SubjectOrganizationalUnit, v.IssuerCommonName, v.IssuerOrganization, v.IssuerOrganizationalUnit, v.CreatedDate, v.ExpiryDate, fmt.Sprintf("%t", v.CertMatched), v.ErrorMsg)
			}
		}
	}

	// get custom record cert info list from cache
	if len(public.Config.CustomRecords) != 0 {
		recordCertInfoCacheKey := public.RecordCertInfo + "_" + public.CustomRecords
		var recordCerts []provider.RecordCert
		recordCertInfoCacheValue, err := public.CertCache.Get(recordCertInfoCacheKey)
		if err != nil {
			logger.Error(fmt.Sprintf("[ %s ] get record list failed: %v", recordCertInfoCacheKey, err))
			return
		}
		err = json.Unmarshal(recordCertInfoCacheValue, &recordCerts)
		if err != nil {
			logger.Error(fmt.Sprintf("[ %s ] json.Unmarshal error: %v", recordCertInfoCacheKey, err))
			return
		}
		for _, v := range recordCerts {
			ch <- prometheus.MustNewConstMetric(c.metrics[public.RecordCertInfo], prometheus.GaugeValue, float64(v.DaysUntilExpiry), v.CloudProvider, v.CloudName, v.DomainName, v.RecordID, v.FullRecord, v.SubjectCommonName, v.SubjectOrganization, v.SubjectOrganizationalUnit, v.IssuerCommonName, v.IssuerOrganization, v.IssuerOrganizationalUnit, v.CreatedDate, v.ExpiryDate, fmt.Sprintf("%t", v.CertMatched), v.ErrorMsg)
		}
	}

	// build info metric
	ch <- prometheus.MustNewConstMetric(
		c.metrics[public.BuildInfo],
		prometheus.GaugeValue,
		1,
		c.version,
		c.gitCommit,
		c.goVersion,
		c.osArch,
		c.buildTime,
	)
}
