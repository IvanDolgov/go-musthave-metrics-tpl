package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/crypto"
)

func main() {
	var (
		privateKeyPath = flag.String("private", "private.key", "Path to save private key")
		publicKeyPath  = flag.String("public", "public.key", "Path to save public key")
		bits           = flag.Int("bits", 2048, "Key size in bits")
	)
	flag.Parse()

	fmt.Println("Generating RSA key pair...")
	fmt.Printf("Private key: %s\n", *privateKeyPath)
	fmt.Printf("Public key: %s\n", *publicKeyPath)
	fmt.Printf("Key size: %d bits\n", *bits)

	if err := crypto.GenerateKeyPair(*privateKeyPath, *publicKeyPath, *bits); err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}

	fmt.Println("Key pair generated successfully!")
}
