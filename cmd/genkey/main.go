package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/crypto"
)

func main() {
	var (
		privateKeyPath string
		publicKeyPath  string
		bits           int
	)

	flag.StringVar(&privateKeyPath, "private", "private.pem", "path to save private key")
	flag.StringVar(&publicKeyPath, "public", "public.pem", "path to save public key")
	flag.IntVar(&bits, "bits", 4096, "key size in bits")
	flag.Parse()

	fmt.Printf("Generating RSA key pair (%d bits)...\n", bits)

	if err := crypto.GenerateKeyPair(privateKeyPath, publicKeyPath, bits); err != nil {
		log.Fatalf("Failed to generate keys: %v", err)
	}

	fmt.Printf("Private key saved to: %s\n", privateKeyPath)
	fmt.Printf("Public key saved to: %s\n", publicKeyPath)
}
