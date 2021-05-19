package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/surajssd/validate-secrets/pkg/webhook"
)

var rootCmd = &cobra.Command{
	Use:   "validate-secrets",
	Short: "Starts a HTTP server, useful for testing MutatingAdmissionWebhook and ValidatingAdmissionWebhook",
	Long: `Starts a HTTP server, useful for testing MutatingAdmissionWebhook and ValidatingAdmissionWebhook.
After deploying it to Kubernetes cluster, the Administrator needs to create a ValidatingWebhookConfiguration
in the Kubernetes cluster to register remote webhook admission controllers.`,
	// Args: cobra.MaximumNArgs(0),
	Run: run,
}

var (
	certFile string
	keyFile  string
	port     int
)

func init() {
	rootCmd.Flags().StringVar(&certFile, "tls-cert-file", "/etc/webhook/cert.pem",
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert).")
	rootCmd.Flags().StringVar(&keyFile, "tls-private-key-file", "/etc/webhook/key.pem",
		"File containing the default x509 private key matching --tls-cert-file.")
	rootCmd.Flags().IntVar(&port, "port", 8443,
		"Secure port that the webhook listens on")

	fs := flag.NewFlagSet("", flag.PanicOnError)
	klog.InitFlags(fs)
	rootCmd.Flags().AddGoFlagSet(fs)
}

func run(cmd *cobra.Command, args []string) {
	sCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		klog.Fatal(err)
	}

	http.HandleFunc("/validate", webhook.ValidateSecretAccess)

	// TODO: Run this on a HTTP port, so readyness can be setup
	http.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("ok")) })

	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{sCert}},
	}

	klog.Info("Starting Server")
	err = server.ListenAndServeTLS("", "")
	if err != nil {
		panic(err)
	}
	klog.Flush()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
