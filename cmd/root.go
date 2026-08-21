package cmd

import (
	"os"
	"runtime/debug"

	"github.com/lilendian0x00/xray-knife/v11/cmd/cfscanner"
	xkexec "github.com/lilendian0x00/xray-knife/v11/cmd/exec"
	"github.com/lilendian0x00/xray-knife/v11/cmd/http"
	"github.com/lilendian0x00/xray-knife/v11/cmd/net"
	"github.com/lilendian0x00/xray-knife/v11/cmd/parse"
	"github.com/lilendian0x00/xray-knife/v11/cmd/proxy"
	"github.com/lilendian0x00/xray-knife/v11/cmd/subs"
	"github.com/lilendian0x00/xray-knife/v11/cmd/webui"
	"github.com/lilendian0x00/xray-knife/v11/database"
	"github.com/lilendian0x00/xray-knife/v11/utils/customlog"
	"github.com/lilendian0x00/xray-knife/v11/utils/xkhome"
	"github.com/spf13/cobra"
)

// version is stamped at build time:
//
//	go build -ldflags "-X github.com/lilendian0x00/xray-knife/v11/cmd.version=11.0.0"
//
// Left as "dev" for a plain `go build`, which then falls back to the module
// version recorded by `go install`.
var version = "dev"

// dbPathOverride backs the persistent --db flag. Empty means "use the default
// under XRAY_KNIFE_HOME (or ~/.xray-knife)".
var dbPathOverride string

// resolveVersion prefers the ldflags-stamped value, then the module version
// baked in by `go install`, so a source build never reports a stale release
// number it was never built from.
func resolveVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	if version != "" {
		return version
	}
	return "dev"
}

// rootCmd is the top-level cobra command.
var rootCmd = &cobra.Command{
	Use:   "xray-knife",
	Short: "Swiss Army Knife for xray-core & sing-box",
	Example: `  # 1. Add a subscription and pull its configs into the local DB.
  #    Fetched links are also written to configs.txt.
  xray-knife subs add --url "https://example.com/sub" --remark "My VPN"
  xray-knife subs fetch --all

  # 2. Test the fetched configs; working ones land in valid.txt, fastest first.
  xray-knife http -f configs.txt

  # 3. Run a local SOCKS proxy on 127.0.0.1:9999 that rotates through them.
  xray-knife proxy inbound -f valid.txt`,
}

// Execute is called by main() to kick everything off.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func addSubcommandPalettes() {
	rootCmd.AddCommand(parse.ParseCmd)
	rootCmd.AddCommand(subs.SubsCmd)
	rootCmd.AddCommand(http.HttpCmd)
	rootCmd.AddCommand(net.NetCmd)
	rootCmd.AddCommand(cfscanner.CFscannerCmd)
	rootCmd.AddCommand(proxy.ProxyCmd)
	rootCmd.AddCommand(webui.WebUICmd)
	rootCmd.AddCommand(xkexec.ExecCmd)
}

// Set up the application's configuration and initialize the database.
func initConfig() {
	dbPath, err := xkhome.DBPath(dbPathOverride)
	if err != nil {
		customlog.Printf(customlog.Failure, "Could not resolve the database path: %v\n", err)
		os.Exit(1)
	}

	// This opens the connection and runs migrations.
	if err := database.InitDB(dbPath); err != nil {
		customlog.Printf(customlog.Failure, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.Version = resolveVersion()

	// -v is verbose in every subcommand; keep the root consistent by putting
	// --version on -V rather than letting the same letter mean two things.
	rootCmd.Flags().BoolP("version", "V", false, "version for xray-knife")
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&dbPathOverride, "db", "",
		"Path to the xray-knife SQLite database (default: $XRAY_KNIFE_HOME/xray-knife.db, else ~/.xray-knife/xray-knife.db)")

	addSubcommandPalettes()
}
