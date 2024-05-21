package accessCode

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/samborkent/uuidv7"
)

func GenerateCode(slug string) string {
	uniqueID := uuidv7.New()

	code := fmt.Sprintf("%s|%s", slug, uniqueID.String())

	// Encoding the string
	encodedString := base64.StdEncoding.EncodeToString([]byte(code))
	fmt.Println("Encoded:", encodedString)

	return encodedString
}

func Decode(code string) (slug, uniqueID string, err error) {
	// Decoding the string
	decodedBytes, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		fmt.Println("Error decoding:", err)
		return "", "", err
	}
	decodedString := string(decodedBytes)
	fmt.Println("Decoded:", decodedString)
	res := strings.Split(decodedString, "|")
	if len(res) != 2 {
		return "", "", fmt.Errorf("not correct format")
	}
	return res[0], res[1], nil
}
