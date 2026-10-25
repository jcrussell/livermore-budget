// Package strictyaml decodes a YAML file that must hold exactly one document
// whose every key the target declares. Callers wrap its errors in their own
// type, keeping err.Error() as the message.
package strictyaml

import (
	"errors"
	"io"

	yaml "go.yaml.in/yaml/v3"
)

var (
	// ErrEmpty is a file holding no document.
	ErrEmpty = errors.New("file is empty")
	// ErrMultipleDocuments is a file holding a second `---` document, refused
	// rather than silently discarded.
	ErrMultipleDocuments = errors.New("file contains more than one YAML document")
)

// Decode decodes the one document r holds into v. A key v does not declare is
// an error, so a misspelled key fails rather than leaving its field zero. It
// returns ErrEmpty, ErrMultipleDocuments or the decoder's own error.
func Decode(r io.Reader, v any) error {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrEmpty
		}
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return ErrMultipleDocuments
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
