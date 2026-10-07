package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"hwops/internal/sandbox"
)

func main() {
	var cfg sandbox.Config
	var tokenFile, job string
	var probe bool
	flag.StringVar(&cfg.Root, "root", "/var/lib/hwops-runner", "private runner journal and files")
	flag.StringVar(&cfg.Image, "image", "", "fixed local image sha256 digest")
	flag.StringVar(&cfg.Listen, "listen", "127.0.0.1:8091", "control listener")
	flag.StringVar(&cfg.TLSCert, "tls-cert", "", "TLS certificate for remote control")
	flag.StringVar(&cfg.TLSKey, "tls-key", "", "TLS private key")
	flag.StringVar(&tokenFile, "token-file", "", "dedicated control token (0600 file)")
	flag.StringVar(&job, "job", "", "internal systemd job identity")
	flag.BoolVar(&probe, "probe", false, "verify isolation and print capability")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if job != "" {
		if err := sandbox.RunJob(cfg.Root, job); err != nil {
			log.Fatal(err)
		}
		return
	}
	if probe {
		capability, err := sandbox.Probe(ctx, cfg)
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(capability)
		return
	}
	info, err := os.Stat(tokenFile)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		log.Fatal("runner requires a private token file")
	}
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		log.Fatal("runner token unavailable")
	}
	cfg.Token = strings.TrimSpace(string(raw))
	if err := sandbox.Serve(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
