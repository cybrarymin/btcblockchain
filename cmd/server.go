/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	CmdBlockStore                  string
	CmdBootstrap                   bool
	CmdBootstrapAddr               string
	CmdChainName                   string
	CmdAuthAccountPass             string
	CmdChainBalance                uint64
	CmdDiscoveryInterval           time.Duration
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "blockchain server side commands",
	Long:  `all the server side components can get run with server command`,
	Run: func(cmd *cobra.Command, args []string) {
		if CmdBootstrap {
			fmt.Print("enter authority account password: ")
			password, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Panicf("\"{\"level\":\"panic\",\"error\":\"%s\" ,\"message\":\"failed to read the authority account password\"}\"", err.Error())
			}
			CmdAuthAccountPass = string(password)
			fmt.Println("")
		}
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
	serverCmd.PersistentFlags().StringVar(&CmdBlockStore, "blockstore-dir", ".blockchain", "block store directory to store genesis and other blocks")
	serverCmd.PersistentFlags().BoolVar(&CmdBootstrap, "bootstrap", false, "is this node a boostrap node or not")
	serverCmd.PersistentFlags().StringVar(&CmdBootstrapAddr, "bootstrap-addr", "", "The boostrap node address for peers")
	serverCmd.PersistentFlags().StringVar(&CmdChainName, "chain", "contoso", "Blockchain name")
	serverCmd.PersistentFlags().Uint64Var(&CmdChainBalance, "chain-balance", 1_000_000_000_000_000_000, "Blockchain balance")
	serverCmd.PersistentFlags().DurationVar(&CmdDiscoveryInterval, "discovery-internal", time.Second*5, "interval that each node tries to perform p2p discovery")
}
