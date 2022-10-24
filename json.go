package gosh

import (
	"encoding/json"
	"io"
)

// JSONDecoderOptions are options to provide to an encoding/json.Decoder
type JSONDecoderOptions struct {
	UseNumber             bool
	DisallowUnknownFields bool
}

// SaveJSON returns a PipeSink which decodes JSON-encoded output and stores it in the value pointed to by v . See encoding/json for rules on how this works.
func SaveJSON(v interface{}, opts ...JSONDecoderOptions) PipeSink {
	return func(r io.Reader) error {
		dec := json.NewDecoder(r)
		for _, opt := range opts {
			if opt.UseNumber {
				dec.UseNumber()
			}
			if opt.DisallowUnknownFields {
				dec.DisallowUnknownFields()
			}
		}
		return dec.Decode(v)
	}
}

// JSONEncoderOptions are options to provide to an encoding/json.Encoder
type JSONEncoderOptions struct {
	IndentString string
	IndentPrefix string
	HTMLEscape   bool
}

// JSONIn returns a PipeSource which provides the JSON encoding of v. See encoding/json for rules on how this works.
func JSONIn(v interface{}, opts ...JSONEncoderOptions) PipeSource {
	return func(w io.Writer) error {
		enc := json.NewEncoder(w)
		for _, opt := range opts {
			if opt.IndentString != "" || opt.IndentPrefix != "" {
				enc.SetIndent(opt.IndentPrefix, opt.IndentString)
			}
			if opt.HTMLEscape {
				enc.SetEscapeHTML(true)
			}
		}
		return enc.Encode(v)
	}
}
