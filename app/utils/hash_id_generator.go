package utils

import (
	"crypto/rand"
	"math/big"
	"strings"
)

type HashIDGenerator struct {
	length int
}

func NewHashIDGenerator(length int) *HashIDGenerator {
	return &HashIDGenerator{length: length}
}

func (g *HashIDGenerator) Generate() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const alphanum = "abcdefghijklmnopqrstuvwxyz0123456789"
	var sb strings.Builder

	// Start with a lowercase letter
	sb.WriteByte(letters[g.cryptoRandInt(len(letters))])

	// Generate the rest of the hash ID
	for i := 1; i < g.length; i++ {
		sb.WriteByte(alphanum[g.cryptoRandInt(len(alphanum))])
	}

	// Ensure the generated ID does not end with a dash
	result := sb.String()
	if result[len(result)-1] == '-' {
		// Replace the last character if it's a dash
		return result[:len(result)-1] + string(letters[g.cryptoRandInt(len(letters))])
	}

	return result
}

func (g *HashIDGenerator) cryptoRandInt(max int) byte {
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic(err) // Handle the error according to your application's needs
	}
	return byte(nBig.Int64())
}
