/*
Copyright © 2024 none
*/
package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"time"
	"xray-knife/detector"
	"xray-knife/xray"
)

var (
	Verbose      bool
	subscribe    string
	conf         string
	parallel     uint16
	subscription []string
)

// testCmd represents the test command
var testCmd = &cobra.Command{
	Use:   "test",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		if subscribe != "" {
			sub := xray.Subscription{
				Url:         subscribe,
				Method:      "GET",
				ConfigLinks: []string{},
			}
			configs, err := sub.FetchAll()
			if err != nil {
				return err
				//os.Exit(1)
			}
			subscription = configs
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {

		ctx := context.Background()

		//parallel, _ := cmd.Flags().GetUint16("parallel")
		println("parallel : ", parallel)

		tester := &detector.Tester{
			Ctx:      ctx,
			Parallel: parallel,
			Timeout:  20 * time.Second,
		}

		tester.Run(subscription)

		fmt.Println("test called")
	},
}

func init() {
	rootCmd.AddCommand(testCmd)

	testCmd.PersistentFlags().StringVarP(&subscribe, "subscribe", "s", "", "subscribe `URL`")
	testCmd.MarkPersistentFlagRequired("subscribe")

	testCmd.PersistentFlags().StringVar(&conf, "conf", "", "load `config`.json file")
	testCmd.PersistentFlags().Uint16VarP(&parallel, "parallel", "p", 50, "concurrency `number`, Range 1-65535")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// testCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// testCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
