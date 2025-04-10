/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

var (
	CmdJaegerHostFlag              string
	CmdJaegerPortFlag              string
	CmdJaegerConnectionTimeout     time.Duration
	CmdSpanExportInterval          time.Duration
	CmdLogLevelFlag                string
	CmdGrpcHost                    string
	CmdGrpcPort                    string
	CmdGrpcGracefulShutdownTimeout time.Duration
	CmdKeyStoreDir                 string
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "blockchain server side commands",
	Long:  `all the server side components can get run with server command`,
	Run: func(cmd *cobra.Command, args []string) {

		cmdMain()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.PersistentFlags().StringVar(&CmdJaegerHostFlag, "jeager-host", "localhost", "Jaeger server address for sending opentelemetry traces")
	serverCmd.PersistentFlags().StringVar(&CmdJaegerPortFlag, "jeager-port", "4318", "Jaeger server port for sending opentelemetry traces")
	serverCmd.PersistentFlags().DurationVar(&CmdJaegerConnectionTimeout, "jeager-conn-timeout", time.Second*5, "connection will fail if it couldn't be established to jaeger host within this time")
	serverCmd.PersistentFlags().DurationVar(&CmdSpanExportInterval, "jeager-trace-exporter-intervals", time.Second*5, "intervals which tracer batch exporter will send the traces to the jeager")
	serverCmd.PersistentFlags().StringVar(&CmdLogLevelFlag, "log-level", "info", "log levels: debug, info, warn, error, fatal, panic, trace, disabled")
	serverCmd.PersistentFlags().StringVar(&CmdGrpcHost, "grpc-listen-address", "0.0.0.0", "grpc server listen addres")
	serverCmd.PersistentFlags().StringVar(&CmdGrpcPort, "grpc-listen-port", "5317", "grpc server port to listen on")
	serverCmd.PersistentFlags().DurationVar(&CmdGrpcGracefulShutdownTimeout, "grpc-shutdown-timeout", time.Second*10, "grpc server graceful shutdown timeout")
	serverCmd.PersistentFlags().StringVar(&CmdKeyStoreDir, "keystore-dir", ".keyStore", "keyStore for storing the accounts private,public key")
}
