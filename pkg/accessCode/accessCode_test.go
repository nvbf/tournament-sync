package accessCode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCode(t *testing.T) {
	slug := "exampleSlug"
	encodedCode := GenerateCode(slug)
	assert.NotEmpty(t, encodedCode, "Encoded code should not be empty")
}

func TestDecode(t *testing.T) {
	// First, generate a code
	slug := "testSlug"
	encodedCode := GenerateCode(slug)

	// Now, decode the encoded code
	decodedSlug, decodedUUID, err := Decode(encodedCode)

	// Check if there are any errors
	assert.Nil(t, err, "Should not have an error during decoding")
	assert.Equal(t, slug, decodedSlug, "Decoded slug should match the original")
	assert.NotEmpty(t, decodedUUID, "Decoded UUID should not be empty")
}

func TestDecode_ErrorHandling(t *testing.T) {
	// Pass an incorrectly formatted string
	_, _, err := Decode("this is not a base64 string")
	assert.NotNil(t, err, "Expected an error for incorrect base64 string")
}
