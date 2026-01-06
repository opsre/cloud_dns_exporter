package cmd

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	"github.com/eryajf/cloud_dns_exporter/pkg/export"
	"github.com/eryajf/cloud_dns_exporter/public/logger"

	"github.com/eryajf/cloud_dns_exporter/public"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var (
	Version   string
	GitCommit string
	BuildTime string
)

func init() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate(fmt.Sprintf(`{{with .Name}}{{printf "%%s version information: " .}}{{end}}
  {{printf "Version:    %%s" .Version}}
  Git Commit: %s
  Go version: %s
  OS/Arch:    %s/%s
  Build Time: %s`, GitCommit, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildTime))
	rootCmd.Flags().BoolP("version", "v", false, "Show version information")
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

var rootCmd = &cobra.Command{
	Use:  "cloud_dns_exporter",
	Long: `cloud_dns_exporter is a tool to export dns records and record cert info from cloud providers.`,
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
			fmt.Println(cmd.VersionTemplate())
			return
		}
		logger.InitLogger("debug")
		public.InitSvc()
		logger.Info("🚀 Start Cloud DNS Exporter, The Metrics Data Is Loading...")
		export.InitCron()
		RunServer()
	},
}

func RunServer() {
	osArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	metrics := export.NewMetrics("", Version, GitCommit, runtime.Version(), osArch, BuildTime)
	registory := prometheus.NewRegistry()
	registory.MustRegister(metrics)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
			<head><title>Cloud DNS Exporter</title></head>
			<body>
			<h1>Cloud DNS Exporter</h1>
			<p><a href='/metrics'>Metrics</a></p>
			<p><a href='https://github.com/eryajf/cloud_dns_exporter'>Source Repo</a></p>
			<p><a href='https://github.com/eryajf'>Create By Eryajf</a></p>
			</body>
			</html>`))
		if err != nil {
			logger.Error("Write Response Error: ", err)
		}
	})
	http.Handle("/metrics", promhttp.HandlerFor(registory, promhttp.HandlerOpts{Registry: registory}))
	port := os.Getenv("PORT")
	if port == "" {
		port = "21798"
	}
	logger.Info("🚀 The Server Listen On Port " + port + ", Enjoy it 🎉")
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
