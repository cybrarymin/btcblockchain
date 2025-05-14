/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var (
	CmdBootstrapGrpcHostFlag string
	CmdBootstrapGrpcPortFlag string
)

// clientCmd represents the client command
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "blockchain client/user side commands",
	Long:  `client side commands are used by the user to do the client side related actions or grpc requests`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdClientMain()
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.PersistentFlags().StringVar(&CmdBootstrapGrpcHostFlag, "bootstrap-grpc-address", "localhost", "bootstrap grpc server listen addres")
	clientCmd.PersistentFlags().StringVar(&CmdBootstrapGrpcPortFlag, "bootstrap-grpc-port", "5317", "bootstrap grpc server port to listen on")
	clientCmd.PersistentFlags().StringVar(&CmdLogLevelFlag, "log-level", "info", "log levels: debug, info, warn, error, fatal, panic, trace, disabled")
}
