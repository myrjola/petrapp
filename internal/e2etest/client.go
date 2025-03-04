package e2etest

import (
	"context"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/descope/virtualwebauthn"
	"github.com/justinas/nosurf"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

type Client struct {
	client        *http.Client
	url           string
	rp            virtualwebauthn.RelyingParty
	authenticator virtualwebauthn.Authenticator
}

// NewClient creates a Webauthn-aware HTTP client.
//
// rpID and rpOrigin should correspond to the Webauthn setup on the server.
func NewClient(url, rpID, rpOrigin string) (*Client, error) {
	jar, err := newUnsafeCookieJar()
	if err != nil {
		return nil, fmt.Errorf("create unsafe cookie jar: %w", err)
	}
	return &Client{
		client:        &http.Client{Jar: jar},
		url:           url,
		rp:            virtualwebauthn.RelyingParty{Name: "Petrapp", ID: rpID, Origin: rpOrigin},
		authenticator: virtualwebauthn.NewAuthenticator(),
	}, nil
}

// WaitForReady calls the specified endpoint until it gets a HTTP 200 Success
// response or until the context is cancelled or the 1-second timeout is reached.
func (c *Client) WaitForReady(ctx context.Context, urlPath string) error {
	timeout := 1 * time.Second
	startTime := time.Now()
	var (
		err  error
		req  *http.Request
		resp *http.Response
	)
	for {
		if req, err = http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			c.url+urlPath,
			nil,
		); err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if resp, err = c.client.Do(req); err == nil {
			if resp.StatusCode == http.StatusOK {
				if err = resp.Body.Close(); err != nil {
					return fmt.Errorf("close response body: %w", err)
				}
				return nil
			}
			if err = resp.Body.Close(); err != nil {
				return fmt.Errorf("close response body: %w", err)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
			if time.Since(startTime) >= timeout {
				return errors.New("timeout waiting for endpoint to be ready")
			}
			time.Sleep(100 * time.Millisecond) //nolint:mnd // 100ms
		}
	}
}

// Get fetches a URL and returns the response.
func (c *Client) Get(ctx context.Context, urlPath string) (*http.Response, error) {
	var (
		err  error
		req  *http.Request
		resp *http.Response
	)
	if req, err = c.newRequestWithContext(ctx, http.MethodGet, urlPath, nil); err != nil {
		return nil, fmt.Errorf("create request with context: %w", err)
	}
	if resp, err = c.client.Do(req); err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}

// GetDoc fetches a URL and returns a goquery document.
func (c *Client) GetDoc(ctx context.Context, urlPath string) (*goquery.Document, error) {
	var (
		err  error
		resp *http.Response
		doc  *goquery.Document
	)
	if resp, err = c.Get(ctx, urlPath); err != nil {
		return nil, fmt.Errorf("client get: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if http.StatusOK != resp.StatusCode {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	if doc, err = goquery.NewDocumentFromReader(resp.Body); err != nil {
		return nil, fmt.Errorf("create document from reader: %w", err)
	}
	return doc, nil
}

// newRequestWithContext creates a new HTTP request to the server that respects the given context.
func (c *Client) newRequestWithContext(
	ctx context.Context,
	method, urlPath string,
	body io.Reader,
) (*http.Request, error) {
	var (
		req *http.Request
		err error
	)
	if req, err = http.NewRequest(method, c.url+urlPath, body); err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return req.WithContext(ctx), nil
}

// Register registers a new WebAuthn credential with the server and returns the front page document.
func (c *Client) Register(ctx context.Context) (*goquery.Document, error) {
	doc, err := c.GetDoc(ctx, "/")
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}

	var (
		registrationStartURLPath = "/api/registration/start"
		csrfToken                string
	)
	if csrfToken, err = c.extractCSRFToken(doc, registrationStartURLPath); err != nil {
		return nil, fmt.Errorf("extract CSRF token: %w", err)
	}
	var attOpts *virtualwebauthn.AttestationOptions
	if attOpts, err = c.startRegistration(ctx, registrationStartURLPath, csrfToken); err != nil {
		return nil, fmt.Errorf("start registration: %w", err)
	}

	var credential *virtualwebauthn.Credential
	if credential, err = c.finishRegistration(ctx, attOpts, csrfToken); err != nil {
		return nil, fmt.Errorf("finish registration: %w", err)
	}

	// At this point, our credential is ready for logging in.
	c.authenticator.AddCredential(*credential)
	// This option is needed for making Passkey login work.
	c.authenticator.Options.UserHandle = []byte(attOpts.UserID)

	if doc, err = c.GetDoc(ctx, "/"); err != nil {
		return nil, fmt.Errorf("get document after registration: %w", err)
	}
	return doc, nil
}

// finishRegistration finishes the registration process and returns the new credential that can be used for logging in.
func (c *Client) finishRegistration(
	ctx context.Context,
	attOpts *virtualwebauthn.AttestationOptions,
	csrfToken string,
) (*virtualwebauthn.Credential, error) {
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(c.rp, c.authenticator, credential, *attOpts)
	var (
		req *http.Request
		err error
	)
	if req, err = c.newRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/registration/finish",
		strings.NewReader(attestationResponse),
	); err != nil {
		return nil, fmt.Errorf("new request with context: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(nosurf.HeaderName, csrfToken)
	var resp *http.Response
	if resp, err = c.client.Do(req); err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close response body: %w", err)
	}
	if http.StatusOK != resp.StatusCode {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return &credential, nil
}

// startRegistration starts the registration process and returns the attestation options needed for finishRegistration.
func (c *Client) startRegistration(
	ctx context.Context,
	registrationStartURLPath string,
	csrfToken string,
) (*virtualwebauthn.AttestationOptions, error) {
	var (
		err error
		req *http.Request
	)
	if req, err = c.newRequestWithContext(ctx, http.MethodPost, registrationStartURLPath, nil); err != nil {
		return nil, fmt.Errorf("new request with context: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(nosurf.HeaderName, csrfToken)
	var resp *http.Response
	if resp, err = c.client.Do(req); err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	if http.StatusOK != resp.StatusCode {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var bodyBytes []byte
	if bodyBytes, err = io.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("read body bytes: %w", err)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close response body: %w", err)
	}
	var attOpts *virtualwebauthn.AttestationOptions
	if attOpts, err = virtualwebauthn.ParseAttestationOptions(string(bodyBytes)); err != nil {
		return nil, fmt.Errorf("parse attestation options: %w", err)
	}
	return attOpts, nil
}

// Login logs in to the server given there is a registered WebAuthn credential and returns the front page document.
func (c *Client) Login(ctx context.Context) (*goquery.Document, error) {
	var (
		doc *goquery.Document
		err error
	)
	if doc, err = c.GetDoc(ctx, "/"); err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}

	var (
		loginStartURLPath = "/api/login/start"
		csrfToken         string
	)
	if csrfToken, err = c.extractCSRFToken(doc, loginStartURLPath); err != nil {
		return nil, fmt.Errorf("extract CSRF token: %w", err)
	}

	var asOpts *virtualwebauthn.AssertionOptions
	if asOpts, err = c.startLogin(ctx, loginStartURLPath, csrfToken); err != nil {
		return nil, fmt.Errorf("start login: %w", err)
	}

	if err = c.finishLogin(ctx, asOpts, csrfToken); err != nil {
		return nil, fmt.Errorf("finish login: %w", err)
	}

	if doc, err = c.GetDoc(ctx, "/"); err != nil {
		return nil, fmt.Errorf("get document after login: %w", err)
	}
	return doc, nil
}

// startLogin starts the login process and returns the assertion options needed for finishLogin.
func (c *Client) startLogin(
	ctx context.Context,
	loginStartURLPath string,
	csrfToken string,
) (*virtualwebauthn.AssertionOptions, error) {
	var (
		req *http.Request
		err error
	)
	if req, err = c.newRequestWithContext(ctx, http.MethodPost, loginStartURLPath, nil); err != nil {
		return nil, fmt.Errorf("new request with context: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(nosurf.HeaderName, csrfToken)
	var resp *http.Response
	if resp, err = c.client.Do(req); err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	if http.StatusOK != resp.StatusCode {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var bodyBytes []byte
	if bodyBytes, err = io.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("read body bytes: %w", err)
	}
	if err = resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close response body: %w", err)
	}
	var asOpts *virtualwebauthn.AssertionOptions
	if asOpts, err = virtualwebauthn.ParseAssertionOptions(string(bodyBytes)); err != nil {
		return nil, fmt.Errorf("parse assertion options: %w", err)
	}
	return asOpts, nil
}

func (c *Client) finishLogin(ctx context.Context, asOpts *virtualwebauthn.AssertionOptions, csrfToken string) error {
	credential := c.authenticator.Credentials[0]
	asResp := virtualwebauthn.CreateAssertionResponse(c.rp, c.authenticator, credential, *asOpts)
	var (
		req *http.Request
		err error
	)
	if req, err = c.newRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/login/finish",
		strings.NewReader(asResp),
	); err != nil {
		return fmt.Errorf("new request with context: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(nosurf.HeaderName, csrfToken)
	var resp *http.Response
	if resp, err = c.client.Do(req); err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	if err = resp.Body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}
	if http.StatusOK != resp.StatusCode {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Logout(ctx context.Context) (*goquery.Document, error) {
	var (
		doc *goquery.Document
		err error
	)
	if doc, err = c.GetDoc(ctx, "/preferences"); err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc, err = c.SubmitForm(ctx, doc, "/api/logout", nil); err != nil {
		return nil, fmt.Errorf("submit form: %w", err)
	}
	return doc, nil
}

func (c *Client) extractCSRFToken(doc *goquery.Document, formActionURLPath string) (string, error) {
	formSelector := fmt.Sprintf("form[action='%s']", formActionURLPath)
	form := doc.Find(formSelector)
	csrfToken, ok := form.Find("input[name=csrf_token]").Attr("value")
	if !ok {
		return "", errors.New("csrf_token not found in form")
	}
	return csrfToken, nil
}

// SubmitForm submits a form in the doc identified with action formActionUrlPath and returns the response document.
// formFields is a map of label text to value. The function will find the input by label and set its value.
func (c *Client) SubmitForm(
	ctx context.Context,
	doc *goquery.Document,
	formActionURLPath string,
	formFields map[string]string,
) (*goquery.Document, error) {
	// Extract CSRF token from the form
	csrfToken, err := c.extractCSRFToken(doc, formActionURLPath)
	if err != nil {
		return nil, fmt.Errorf("extract CSRF token: %w", err)
	}

	// Build form data
	formData := neturl.Values{}
	formData.Add("csrf_token", csrfToken)

	var form *goquery.Selection
	if form, err = FindForm(doc, formActionURLPath); err != nil {
		return nil, fmt.Errorf("find form: %w", err)
	}

	// Find form inputs based on their labels
	for labelText, value := range formFields {
		var input *goquery.Selection
		if input, err = FindInputForLabel(form, labelText); err != nil {
			return nil, fmt.Errorf("find input for label: %w", err)
		}

		name, exists := input.Attr("name")
		if !exists {
			return nil, fmt.Errorf("input has no name attribute (label: %s, form_action: %s)",
				labelText, formActionURLPath)
		}

		formData.Add(name, value)
	}

	// Submit the form
	data := strings.NewReader(formData.Encode())
	req, err := c.newRequestWithContext(ctx, http.MethodPost, formActionURLPath, data)
	if err != nil {
		return nil, fmt.Errorf("new request with context: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if http.StatusOK != resp.StatusCode {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response
	newDoc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("create document from reader: %w", err)
	}
	newDoc.Url = resp.Request.URL
	return newDoc, nil
}
