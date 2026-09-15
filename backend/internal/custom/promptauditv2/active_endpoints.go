package promptauditv2

import (
	"fmt"
	"sort"
	"time"
)

// buildActiveEndpoints returns a priority-ordered immutable call list from
// prevalidated endpoint configs. API keys remain in the runtime snapshot only.
func buildActiveEndpoints(endpoints []EndpointConfig) ([]ActiveEndpoint, error) {
	active := make([]ActiveEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if !endpoint.Enabled {
			continue
		}
		endpointURL, err := normalizeChatCompletionsURL(endpoint.BaseURL)
		if err != nil {
			return nil, invalidConfig(fmt.Sprintf("endpoint %q base_url is invalid", endpoint.ID))
		}
		active = append(active, ActiveEndpoint{
			ID: endpoint.ID, Name: endpoint.Name, URL: endpointURL, APIKey: endpoint.APIKey,
			Model: endpoint.Model, Priority: endpoint.Priority,
			Timeout: time.Duration(endpoint.TimeoutMS) * time.Millisecond, Order: endpoint.Order,
		})
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].Priority == active[j].Priority {
			return active[i].Order < active[j].Order
		}
		return active[i].Priority > active[j].Priority
	})
	return active, nil
}
