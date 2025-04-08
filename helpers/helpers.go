package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

func JsonMarshaller(ctx context.Context, data interface{}) ([]byte, error) {
	_, span := otel.Tracer("JsonMarshaller.Tracer").Start(ctx, "JsonMarshaller.Span")
	defer span.End()
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(data)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal the provided data")
		return nil, err
	}
	return b.Bytes(), nil
}

func JsonUnMarshaller[T any](ctx context.Context, jsonData []byte) (T, error) {
	_, span := otel.Tracer("JsonMarshaller.Tracer").Start(ctx, "JsonMarshaller.Span")
	defer span.End()

	var output T
	b := bytes.NewReader(jsonData)
	err := json.NewDecoder(b).Decode(&output)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal the provided data")
	}
	return output, err
}

func BackGroundJob(logger *zerolog.Logger, panicErrorMsg string, fn func()) {
	go func() {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				pErr := errors.New(fmt.Sprintln(panicErr))
				logger.Error().Stack().Err(pErr).Msg(panicErrorMsg)
			}
		}()
		fn()
	}()
}
