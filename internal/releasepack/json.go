package releasepack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func decodeStrictJSON(raw []byte, destination any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: multiple JSON values", label)
		}
		return fmt.Errorf("decode %s: malformed trailing JSON: %w", label, err)
	}
	return nil
}
