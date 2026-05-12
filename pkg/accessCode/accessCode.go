package accessCode

import (
	"encoding/base64"
	"fmt"
	"strings"

	log "github.com/nvbf/tournament-sync/pkg/cloudlog"
)

func GenerateCode(slug, secret string) string {
	code := fmt.Sprintf("%s|%s", slug, secret)

	// Encoding the string
	encodedString := base64.StdEncoding.EncodeToString([]byte(code))
	log.Printf("Encoded: %s", encodedString)

	return encodedString
}

func Decode(code string) (slug, uniqueID string, err error) {
	// Decoding the string
	decodedBytes, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		log.Printf("Error decoding: %v", err)
		return "", "", err
	}
	decodedString := string(decodedBytes)
	log.Printf("Decoded: %s", decodedString)
	res := strings.Split(decodedString, "|")
	if len(res) != 2 {
		return "", "", fmt.Errorf("not correct format")
	}
	return res[0], res[1], nil
}
