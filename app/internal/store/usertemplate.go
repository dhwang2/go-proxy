package store

// UserRouteTemplates is the top-level structure for user-route-templates.json.
type UserRouteTemplates struct {
	Templates map[string][]TemplateRule `json:"templates,omitempty"`
}

// TemplateRule is a single rule within a routing template.
type TemplateRule struct {
	Type     string `json:"type"`
	Outbound string `json:"outbound"`
	Domains  string `json:"domains,omitempty"`
}

// NewUserRouteTemplates returns an initialized empty UserRouteTemplates.
func NewUserRouteTemplates() *UserRouteTemplates {
	return &UserRouteTemplates{
		Templates: make(map[string][]TemplateRule),
	}
}
