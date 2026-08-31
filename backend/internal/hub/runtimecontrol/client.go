package runtimecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"measix/platform/internal/wire/relaycontrolapi"
)

type RelayClient interface {
	Apply(context.Context, relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error)
	Status(context.Context) (relaycontrolapi.ControlStatus, error)
}

type RelayHTTPError struct {
	StatusCode int
	Code       string
}

func (e *RelayHTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("relay returned status %d (%s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("relay returned status %d", e.StatusCode)
}

func relayValidationRejected(err error) (string, bool) {
	var relayErr *RelayHTTPError
	if !errors.As(err, &relayErr) {
		return "", false
	}
	if relayErr.StatusCode == http.StatusUnprocessableEntity && relayErr.Code == "invalid_runtime_control" {
		return relayErr.Code, true
	}
	return "", false
}

type HTTPRelayClient struct {
	baseURL      string
	serviceToken string
	client       *http.Client
}

func NewHTTPRelayClient(baseURL, serviceToken string, client *http.Client) *HTTPRelayClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPRelayClient{baseURL: strings.TrimRight(baseURL, "/"), serviceToken: serviceToken, client: client}
}

func (c *HTTPRelayClient) Apply(ctx context.Context, state relaycontrolapi.RuntimeControlState) (relaycontrolapi.ControlAck, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/internal/v1/control/state", bytes.NewReader(payload))
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.serviceToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &problem)
		return relaycontrolapi.ControlAck{}, &RelayHTTPError{StatusCode: response.StatusCode, Code: problem.Code}
	}
	var ack relaycontrolapi.ControlAck
	if err := json.NewDecoder(response.Body).Decode(&ack); err != nil {
		return relaycontrolapi.ControlAck{}, err
	}
	return ack, nil
}

func (c *HTTPRelayClient) Status(ctx context.Context) (relaycontrolapi.ControlStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/v1/control/status", nil)
	if err != nil {
		return relaycontrolapi.ControlStatus{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.serviceToken)
	response, err := c.client.Do(request)
	if err != nil {
		return relaycontrolapi.ControlStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &problem)
		return relaycontrolapi.ControlStatus{}, &RelayHTTPError{StatusCode: response.StatusCode, Code: problem.Code}
	}
	var status relaycontrolapi.ControlStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return relaycontrolapi.ControlStatus{}, err
	}
	return status, nil
}
