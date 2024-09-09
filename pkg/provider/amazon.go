package provider

import (
	"encoding/json"
	"fmt"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/route53domains"
	domainTypes "github.com/aws/aws-sdk-go-v2/service/route53domains/types"
	"github.com/eryajf/cloud_dns_exporter/public"
	"github.com/golang-module/carbon/v2"
	"golang.org/x/net/context"
	"strings"
	"sync"
	"time"
)

type AmazonDNS struct {
	account public.Account
	client  *route53.Client
}

const region = "us-east-1"

func NewAwsDnsClient(secretID string, secretKey string) *route53.Client {
	return route53.New(route53.Options{
		Credentials:      aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(secretID, secretKey, "")),
		Region:           region,
		RetryMaxAttempts: 3,
	})
}

func NewAwsDns(account public.Account) *AmazonDNS {
	client := NewAwsDnsClient(account.SecretID, account.SecretKey)
	return &AmazonDNS{
		account: account,
		client:  client,
	}
}

func NewAwsDomainClient(secretID string, secretKey string) *route53domains.Client {
	return route53domains.New(route53domains.Options{
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(secretID, secretKey, "")),
		Region:      region,
	})
}

func (a *AmazonDNS) ListDomains() ([]Domain, error) {
	ad := NewAwsDns(public.Account{
		CloudProvider: a.account.CloudProvider,
		CloudName:     a.account.CloudName,
		SecretID:      a.account.SecretID,
		SecretKey:     a.account.SecretKey,
	})
	a.client = ad.client
	var dataObj []Domain
	domains, err := a.getDomainList()
	if err != nil {
		return nil, err
	}
	domainNames, err := a.getDomainNameList()
	if err != nil {
		return nil, err
	}
	for _, domain := range domains {
		domainName := strings.TrimSuffix(tea.StringValue(domain.Name), ".")
		domainCreateAndExpiryDate := a.getDomainCreateAndExpiryDate(domainNames, domainName)
		dataObj = append(dataObj, Domain{
			CloudProvider:   a.account.CloudProvider,
			CloudName:       a.account.CloudName,
			DomainID:        strings.TrimPrefix(tea.StringValue(domain.Id), "/"),
			DomainName:      fmt.Sprintf(domainName),
			DomainRemark:    tea.StringValue(nil),
			DomainStatus:    "enable",
			CreatedDate:     domainCreateAndExpiryDate.CreatedDate,
			ExpiryDate:      domainCreateAndExpiryDate.ExpiryDate,
			DaysUntilExpiry: domainCreateAndExpiryDate.DaysUntilExpiry,
		})
	}
	return dataObj, nil
}

func (a *AmazonDNS) ListRecords() ([]Record, error) {
	var (
		dataObj []Record
		domains []Domain
		wg      sync.WaitGroup
		mu      sync.Mutex
	)
	ad := NewAwsDns(public.Account{
		CloudProvider: a.account.CloudProvider,
		CloudName:     a.account.CloudName,
		SecretID:      a.account.SecretID,
		SecretKey:     a.account.SecretKey,
	})
	a.client = ad.client
	rst, err := public.Cache.Get(public.DomainList + "_" + a.account.CloudProvider + "_" + a.account.CloudName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(rst, &domains)
	if err != nil {
		return nil, err
	}
	results := make(map[string][]types.ResourceRecordSet)
	ticker := time.NewTicker(100 * time.Millisecond)
	num := 0
	for _, domain := range domains {
		wg.Add(1)
		// aws 接口并发限制
		time.Sleep(1 * time.Second)
		num++
		go func(domain Domain) {
			defer wg.Done()
			<-ticker.C
			records, err := a.getRecordList(domain.DomainID)
			if err != nil {
				fmt.Printf("get record list failed: %s\n", err)
			}
			mu.Lock()
			results[domain.DomainName] = records
			mu.Unlock()
		}(domain)
		if num >= 2 {
			break
		}
	}
	wg.Wait()
	for domain, record := range results {
		for _, record := range record {
			recordInfo := Record{
				CloudProvider: a.account.CloudProvider,
				CloudName:     a.account.CloudName,
				DomainName:    domain,
				RecordID:      tea.StringValue(record.SetIdentifier),
				RecordType:    fmt.Sprintf("%s", record.Type),
				RecordName:    tea.StringValue(record.Name),
				RecordTTL:     fmt.Sprintf("%d", *record.TTL),
				RecordWeight:  fmt.Sprintf("%d", record.Weight),
				RecordStatus:  oneStatus("enable"),
				RecordRemark:  tea.StringValue(nil),
				UpdateTime:    carbon.CreateFromTimestampMilli(tea.Int64Value(nil)).ToDateTimeString(),
				FullRecord:    tea.StringValue(record.Name),
			}
			if record.ResourceRecords != nil {
				for _, record := range record.ResourceRecords {
					recordInfo.RecordValue = tea.StringValue(record.Value)
					dataObj = append(dataObj, recordInfo)
				}
			} else {
				fmt.Println("record value is nil", record)
			}
		}
	}
	return dataObj, nil
}

// https://docs.aws.amazon.com/Route53/latest/APIReference/API_ListHostedZones.html
// getDomainList 获取托管区域解析域名列表
func (a *AmazonDNS) getDomainList() (rst []types.HostedZone, err error) {
	client := NewAwsDnsClient(a.account.SecretID, a.account.SecretKey)
	var Marker *string
	for {
		output, err := client.ListHostedZones(context.Background(), &route53.ListHostedZonesInput{
			Marker: Marker,
		})
		if err != nil {
			return nil, err
		}
		for _, zone := range output.HostedZones {
			rst = append(rst, zone)
		}
		if output.NextMarker == nil {
			break
		}
		Marker = output.NextMarker
	}
	return
}

// https://docs.aws.amazon.com/Route53/latest/APIReference/API_ListResourceRecordSets.html
// getRecordList 获取解析记录
func (a *AmazonDNS) getRecordList(domainId string) (rst []types.ResourceRecordSet, err error) {
	client := NewAwsDnsClient(a.account.SecretID, a.account.SecretKey)
	var startRecordIdentifier *string
	var startRecordType types.RRType
	var startRecordName *string
	for {
		output, err := client.ListResourceRecordSets(context.Background(), &route53.ListResourceRecordSetsInput{
			HostedZoneId:          tea.String(domainId),
			StartRecordIdentifier: startRecordIdentifier,
			StartRecordType:       startRecordType,
			StartRecordName:       startRecordName,
		})
		if err != nil {
			return nil, err
		}
		rst = append(rst, output.ResourceRecordSets...)
		if output.IsTruncated {
			startRecordIdentifier = output.NextRecordIdentifier
			startRecordType = output.NextRecordType
			startRecordName = output.NextRecordName
		} else {
			break
		}
	}
	return
}

// https://docs.aws.amazon.com/Route53/latest/APIReference/API_domains_ListDomains.html
// getDomainNameList 获取域名列表
func (a *AmazonDNS) getDomainNameList() (rst []domainTypes.DomainSummary, err error) {
	client := NewAwsDomainClient(a.account.SecretID, a.account.SecretKey)
	var Marker *string
	for {
		output, err := client.ListDomains(context.Background(), &route53domains.ListDomainsInput{
			Marker: Marker,
		})
		if err != nil {
			return nil, err
		}
		for _, domain := range output.Domains {
			rst = append(rst, domain)
		}
		if output.NextPageMarker == nil {
			break
		}
		Marker = output.NextPageMarker
	}
	return
}

// 域名详情接口 https://docs.aws.amazon.com/Route53/latest/APIReference/API_domains_GetDomainDetail.html
// getDomainCreateAndExpiryDate 获取域名创建时间、过期时间, 通过域名详情获取
func (a *AmazonDNS) getDomainCreateAndExpiryDate(domainList []domainTypes.DomainSummary, domainName string) (d Domain) {
	client := NewAwsDomainClient(a.account.SecretID, a.account.SecretKey)
	for _, domain := range domainList {
		if tea.StringValue(domain.DomainName) == domainName {
			domainDetail, err := client.GetDomainDetail(context.Background(), &route53domains.GetDomainDetailInput{
				DomainName: tea.String(domainName),
			})
			if err != nil {
				return
			}
			d.CreatedDate = domainDetail.CreationDate.String()
			d.ExpiryDate = domainDetail.ExpirationDate.String()
			if d.ExpiryDate != "" {
				d.DaysUntilExpiry = carbon.Now().DiffInDays(carbon.Parse(d.ExpiryDate))
			}
			break
		}
	}
	return
}
