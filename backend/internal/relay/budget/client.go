package budget

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"measix/platform/internal/wire/usageingestapi"
)

var ErrUnavailable = errors.New("budget service unavailable")

const (
	defaultMaxIdleConnections        = 256
	defaultMaxIdleConnectionsPerHost = 128
)

type Client interface {
	Admit(context.Context, usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error)
	Start(context.Context, string, usageingestapi.BudgetLifecycleEvent) error
	Release(context.Context, string, usageingestapi.BudgetReleaseRequest) error
}

type HTTPClient struct {
	client *usageingestapi.ClientWithResponses
}

func NewHTTPClient(baseURL, token string, client *http.Client) (*HTTPClient, error) {
	baseURL, token = strings.TrimRight(strings.TrimSpace(baseURL), "/"), strings.TrimSpace(token)
	if baseURL == "" || token == "" {
		return nil, fmt.Errorf("budget client requires Hub URL and service credential")
	}
	if client == nil {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			transport = &http.Transport{}
		}
		transport = transport.Clone()
		transport.ForceAttemptHTTP2 = true
		if transport.MaxIdleConns < defaultMaxIdleConnections {
			transport.MaxIdleConns = defaultMaxIdleConnections
		}
		if transport.MaxIdleConnsPerHost < defaultMaxIdleConnectionsPerHost {
			transport.MaxIdleConnsPerHost = defaultMaxIdleConnectionsPerHost
		}
		client = &http.Client{Transport: transport, Timeout: 10 * time.Second}
	}
	generated, err := usageingestapi.NewClientWithResponses(baseURL,
		usageingestapi.WithHTTPClient(client),
		usageingestapi.WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
			request.Header.Set("Authorization", "Bearer "+token)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return &HTTPClient{client: generated}, nil
}

func (c *HTTPClient) Admit(ctx context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	if c == nil || c.client == nil {
		return usageingestapi.BudgetAdmissionDecision{}, nil, ErrUnavailable
	}
	response, err := c.client.AdmitBudgetWithResponse(ctx, input)
	if err != nil {
		return usageingestapi.BudgetAdmissionDecision{}, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if response.JSON200 != nil && response.StatusCode() == http.StatusOK {
		return *response.JSON200, nil, nil
	}
	if problem := admitProblem(response); problem != nil {
		return usageingestapi.BudgetAdmissionDecision{}, problem, nil
	}
	return usageingestapi.BudgetAdmissionDecision{}, nil, fmt.Errorf("%w: unexpected Hub status %d", ErrUnavailable, response.StatusCode())
}

func (c *HTTPClient) Start(ctx context.Context, requestID string, input usageingestapi.BudgetLifecycleEvent) error {
	if c == nil || c.client == nil {
		return ErrUnavailable
	}
	response, err := c.client.MarkBudgetRequestStartedWithResponse(ctx, requestID, input)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if response.StatusCode() == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("start budget request %s: Hub status %d", requestID, response.StatusCode())
}

func (c *HTTPClient) Release(ctx context.Context, requestID string, input usageingestapi.BudgetReleaseRequest) error {
	if c == nil || c.client == nil {
		return ErrUnavailable
	}
	response, err := c.client.ReleaseBudgetRequestWithResponse(ctx, requestID, input)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if response.StatusCode() == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("release budget request %s: Hub status %d", requestID, response.StatusCode())
}

func admitProblem(response *usageingestapi.AdmitBudgetResponse) *usageingestapi.Problem {
	if response == nil {
		return nil
	}
	switch response.StatusCode() {
	case http.StatusUnauthorized:
		return response.ApplicationproblemJSON401
	case http.StatusForbidden:
		return response.ApplicationproblemJSON403
	case http.StatusConflict:
		return response.ApplicationproblemJSON409
	case http.StatusUnprocessableEntity:
		return response.ApplicationproblemJSON422
	case http.StatusTooManyRequests:
		return response.ApplicationproblemJSON429
	case http.StatusServiceUnavailable:
		return response.ApplicationproblemJSON503
	default:
		return nil
	}
}
