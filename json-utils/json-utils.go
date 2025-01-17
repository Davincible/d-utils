package json-utils

import (
	"encoding/json"
	"fmt"
	"io"
)

// Decode decodes JSON data into a struct.
func Decode[T any](data []byte) (T, error) {
	var v T

	if len(data) == 0 {
		return v, fmt.Errorf("no JSON data provided")
	}

	truncatedBody := string(data)
	if len(truncatedBody) > 20 {
		truncatedBody = truncatedBody[20:]
	}

	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("decoding JSON (%s): %w", truncatedBody, err)
	}

	return v, nil
}

// DecodeStr decodes JSON data into a struct.
func DecodeStr[T any](data string) (T, error) {
	return Decode[T]([]byte(data))
}

func DecodeReqBody[T any](reqBody io.ReadCloser) (T, error) {
	defer reqBody.Close()

	var zero T

	body, err := io.ReadAll(reqBody)
	if err != nil {
		return zero, fmt.Errorf("reading request body: %w", err)
	}

	fmt.Println("BODY:", string(body))

	return Decode[T](body)
}
