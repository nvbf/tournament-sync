package accessCode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCode(t *testing.T) {
	slug := "exampleSlug"
	uuid := "stringsy"
	encodedCode := GenerateCode(slug, uuid)
	assert.NotEmpty(t, encodedCode, "Encoded code should not be empty")
}

func TestDecode(t *testing.T) {
	// First, generate a code
	slug := "testSlug"
	uuid := "stringsy"
	encodedCode := GenerateCode(slug, uuid)

	// Now, decode the encoded code
	decodedSlug, decodedUUID, err := Decode(encodedCode)

	// Check if there are any errors
	assert.Nil(t, err, "Should not have an error during decoding")
	assert.Equal(t, slug, decodedSlug, "Decoded slug should match the original")
	assert.Equal(t, uuid, decodedUUID, "Decoded UUID should match the original")
}

func TestDecode_ErrorHandling(t *testing.T) {
	// Pass an incorrectly formatted string
	_, _, err := Decode("this is not a base64 string")
	assert.NotNil(t, err, "Expected an error for incorrect base64 string")
}
