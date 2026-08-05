package assistantapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"writers.studio/internal/assistant"
	"writers.studio/internal/config"
	"writers.studio/internal/llm"
	"writers.studio/internal/store"
)

type ProviderRegistry struct {
	config        config.Config
	store         *store.Store
	credentials   *llm.CredentialBox
	systemMu      sync.Mutex
	systemChecked bool
	systemReport  llm.ProbeReport
}

func NewProviderRegistry(
	cfg config.Config,
	dataStore *store.Store,
) (*ProviderRegistry, error) {
	var credentials *llm.CredentialBox
	if cfg.LLMCredentialKey != "" {
		box, err := llm.NewCredentialBox(cfg.LLMCredentialKey)
		if err != nil {
			return nil, err
		}
		credentials = box
	}
	return &ProviderRegistry{
		config: cfg, store: dataStore, credentials: credentials,
	}, nil
}

func (r *ProviderRegistry) ConnectionsEnabled() bool {
	return r != nil && r.credentials != nil
}

// RequestTimeoutMS exposes the single operator-controlled timeout used for
// system and user-provided connections alike. Provider records retain the
// value only for API/schema compatibility; they never override this policy.
func (r *ProviderRegistry) RequestTimeoutMS() int {
	return int(r.requestTimeout() / time.Millisecond)
}

func (r *ProviderRegistry) requestTimeout() time.Duration {
	if r == nil || r.config.LLMRequestTimeout <= 0 {
		return 10 * time.Minute
	}
	return r.config.LLMRequestTimeout
}

func (r *ProviderRegistry) SystemProviderRef(
	ctx context.Context,
	probe bool,
) (assistant.ProviderRef, bool) {
	reference, _, configured := r.SystemProviderReport(ctx, probe)
	return reference, configured
}

func (r *ProviderRegistry) SystemProviderReport(
	ctx context.Context,
	probe bool,
) (assistant.ProviderRef, llm.ProbeReport, bool) {
	if r == nil || !r.hasSystemProvider() {
		return assistant.ProviderRef{}, llm.ProbeReport{}, false
	}
	report := llm.ProbeReport{
		Status: "unchecked",
		Capabilities: assistant.CapabilitySnapshot{
			SupportedDialects: []string{r.config.LLMDialect},
			CheckedWithModel:  r.config.LLMModel,
		},
		Detail: "Совместимость будет проверена при первом запуске",
	}
	if probe {
		r.systemMu.Lock()
		if !r.systemChecked {
			provider, dialect, err := r.systemProvider(ctx)
			if err != nil {
				r.systemReport = llm.ProbeReport{
					Status: "unavailable",
					Capabilities: assistant.CapabilitySnapshot{
						SupportedDialects: []string{r.config.LLMDialect},
						CheckedWithModel:  r.config.LLMModel,
					},
					Detail: "Системный провайдер недоступен",
				}
			} else {
				r.systemReport = probeProviderCompatibility(
					ctx,
					provider,
					dialect,
					r.config.LLMModel,
				)
			}
			r.systemChecked = true
		}
		report = r.systemReport
		r.systemMu.Unlock()
	}
	return assistant.ProviderRef{
		Kind:         assistant.ProviderSystem,
		Name:         "Системный",
		Model:        r.config.LLMModel,
		Dialect:      assistant.ProviderDialect(r.config.LLMDialect),
		Capabilities: report.Capabilities,
	}, report, true
}

func (r *ProviderRegistry) RefreshSystemProviderReport(
	ctx context.Context,
) (assistant.ProviderRef, llm.ProbeReport, bool) {
	if r == nil {
		return assistant.ProviderRef{}, llm.ProbeReport{}, false
	}
	r.systemMu.Lock()
	r.systemChecked = false
	r.systemReport = llm.ProbeReport{}
	r.systemMu.Unlock()
	return r.SystemProviderReport(ctx, true)
}

func (r *ProviderRegistry) ProviderForRun(
	ctx context.Context,
	ownerID string,
	run assistant.Run,
) (llm.Provider, error) {
	if r == nil {
		return nil, errors.New("provider registry is unavailable")
	}
	switch run.Provider.Kind {
	case assistant.ProviderSystem:
		provider, dialect, err := r.systemProvider(ctx)
		if err != nil {
			return nil, err
		}
		if dialect != run.Provider.Dialect || run.Provider.Model != r.config.LLMModel {
			return nil, errors.New("system provider configuration changed after run creation")
		}
		return provider, nil
	case assistant.ProviderConnection:
		if r.store == nil {
			return nil, errors.New("provider connection store is unavailable")
		}
		if run.Provider.ConnectionID == nil {
			return nil, errors.New("provider connection ID is missing")
		}
		connection, err := r.store.GetProviderConnection(
			ctx,
			ownerID,
			*run.Provider.ConnectionID,
		)
		if err != nil {
			return nil, err
		}
		if connection.Model != run.Provider.Model ||
			connection.Dialect != run.Provider.Dialect ||
			!connection.Capabilities.CanRunAssistant() {
			return nil, errors.New("provider connection no longer matches the pinned run")
		}
		return r.providerForConnection(ctx, connection)
	default:
		return nil, errors.New("unknown provider kind")
	}
}

func (r *ProviderRegistry) ProbeConnection(
	ctx context.Context,
	connection store.ProviderConnectionRecord,
	apiKey string,
	headers map[string]string,
) (llm.ProbeReport, assistant.ProviderDialect, error) {
	if r == nil {
		return llm.ProbeReport{}, "", errors.New("provider registry is unavailable")
	}
	dialects := []assistant.ProviderDialect{connection.Dialect}
	if connection.Dialect == assistant.DialectAuto {
		dialects = []assistant.ProviderDialect{
			assistant.DialectResponses,
			assistant.DialectChatCompletions,
		}
	}
	var lastReport llm.ProbeReport
	for _, dialect := range dialects {
		provider, err := r.newProvider(
			ctx,
			connection.BaseURL,
			apiKey,
			connection.Model,
			dialect,
			headers,
			connection.AllowPrivateEndpoint,
		)
		if err != nil {
			return llm.ProbeReport{}, "", err
		}
		report := probeProviderCompatibility(ctx, provider, dialect, connection.Model)
		lastReport = report
		if report.Capabilities.CanRunAssistant() {
			return report, dialect, nil
		}
	}
	return lastReport, dialects[len(dialects)-1], nil
}

func (r *ProviderRegistry) ProbeStoredConnection(
	ctx context.Context,
	connection store.ProviderConnectionRecord,
) (llm.ProbeReport, error) {
	if r == nil {
		return llm.ProbeReport{}, errors.New("provider registry is unavailable")
	}
	provider, err := r.providerForConnection(ctx, connection)
	if err != nil {
		return llm.ProbeReport{}, err
	}
	return probeProviderCompatibility(
		ctx,
		provider,
		connection.Dialect,
		connection.Model,
	), nil
}

func probeProviderCompatibility(
	ctx context.Context,
	provider llm.Provider,
	dialect assistant.ProviderDialect,
	model string,
) llm.ProbeReport {
	const attempts = 2
	var report llm.ProbeReport
	for attempt := range attempts {
		report = llm.Probe(ctx, provider, dialect, model)
		if report.Capabilities.CanRunAssistant() {
			if attempt > 0 {
				report.Capabilities.Limitations = append(
					report.Capabilities.Limitations,
					"Совместимость подтвердилась со второй попытки; провайдер может отвечать нестабильно",
				)
				report.Status = "limited"
				report.Detail += "; совместимость подтвердилась со второй попытки"
			}
			return report
		}
		if ctx.Err() != nil {
			return report
		}
	}
	return report
}

func (r *ProviderRegistry) EncryptCredential(
	connectionID string,
	apiKey string,
) (llm.EncryptedSecret, error) {
	if r == nil || r.credentials == nil {
		return llm.EncryptedSecret{}, llm.ErrCredentialKeyMissing
	}
	return r.credentials.Encrypt(apiKey, []byte(connectionID))
}

func (r *ProviderRegistry) EncryptHeaders(
	connectionID string,
	headers map[string]string,
) ([]byte, []byte, error) {
	if len(headers) == 0 {
		return nil, nil, nil
	}
	if r == nil || r.credentials == nil {
		return nil, nil, llm.ErrCredentialKeyMissing
	}
	encoded, err := json.Marshal(headers)
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := r.credentials.Encrypt(
		string(encoded),
		[]byte(connectionID+":headers"),
	)
	if err != nil {
		return nil, nil, err
	}
	return encrypted.Ciphertext, encrypted.Nonce, nil
}

func (r *ProviderRegistry) systemProvider(
	ctx context.Context,
) (llm.Provider, assistant.ProviderDialect, error) {
	if r == nil || !r.hasSystemProvider() {
		return nil, "", errors.New("system LLM provider is not configured")
	}
	dialect := assistant.ProviderDialect(r.config.LLMDialect)
	if r.config.LLMAdapter == "fixture" {
		return llm.NewFixtureProvider(), dialect, nil
	}
	provider, err := r.newProvider(
		ctx,
		r.config.LLMBaseURL,
		r.config.LLMAPIKey,
		r.config.LLMModel,
		dialect,
		nil,
		r.config.LLMAllowPrivate,
	)
	return provider, dialect, err
}

func (r *ProviderRegistry) hasSystemProvider() bool {
	return r != nil &&
		(r.config.LLMAdapter == "openai_compatible" || r.config.LLMAdapter == "fixture")
}

func (r *ProviderRegistry) providerForConnection(
	ctx context.Context,
	connection store.ProviderConnectionRecord,
) (llm.Provider, error) {
	if r.credentials == nil {
		return nil, llm.ErrCredentialKeyMissing
	}
	apiKey, err := r.credentials.Decrypt(
		connection.EncryptedAPIKey,
		connection.APIKeyNonce,
		[]byte(connection.ID),
	)
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	if len(connection.EncryptedHeaders) > 0 {
		encoded, err := r.credentials.Decrypt(
			connection.EncryptedHeaders,
			connection.HeadersNonce,
			[]byte(connection.ID+":headers"),
		)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &headers); err != nil {
			return nil, errors.New("encrypted provider headers are invalid")
		}
	}
	return r.newProvider(
		ctx,
		connection.BaseURL,
		apiKey,
		connection.Model,
		connection.Dialect,
		headers,
		connection.AllowPrivateEndpoint,
	)
}

func (r *ProviderRegistry) newProvider(
	ctx context.Context,
	rawBaseURL string,
	apiKey string,
	model string,
	dialect assistant.ProviderDialect,
	headers map[string]string,
	requestPrivate bool,
) (llm.Provider, error) {
	allowPrivate := requestPrivate && r.config.LLMAllowPrivate
	policy := llm.EndpointPolicy{AllowPrivate: allowPrivate}
	baseURL, err := policy.Validate(ctx, rawBaseURL)
	if err != nil {
		return nil, err
	}
	return llm.NewOpenAICompatible(llm.OpenAICompatibleConfig{
		BaseURL: baseURL, APIKey: apiKey, Model: model, Dialect: dialect,
		HTTPClient: policy.HTTPClient(r.requestTimeout()), Headers: headers,
	})
}

func NormalizeProviderBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid provider base URL")
	}
	parsed.Path = stringsTrimRightSlash(parsed.Path)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func stringsTrimRightSlash(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}

var _ ProviderResolver = (*ProviderRegistry)(nil)
var _ llm.Provider = (*llm.OpenAICompatible)(nil)
