package proposal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func canonicalEncode(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func cloneValues[T any](values []T) []T {
	if len(values) == 0 {
		return []T{}
	}
	return append([]T(nil), values...)
}

func compareRevisionKeys(left, right revisionKeyWire) int {
	if left.ArtifactID < right.ArtifactID {
		return -1
	}
	if left.ArtifactID > right.ArtifactID {
		return 1
	}
	if left.RevisionID < right.RevisionID {
		return -1
	}
	if left.RevisionID > right.RevisionID {
		return 1
	}
	return 0
}
