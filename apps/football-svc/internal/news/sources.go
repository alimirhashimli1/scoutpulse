package news

import "log/slog"

// SourceType selects the adapter that reads a source.
type SourceType string

const (
	SourceRSS      SourceType = "rss"
	SourceNewsData SourceType = "newsdata"
)

// Source is one entry in the registry.
//
// A source is fetched only when it is Enabled AND CommercialUseVerified. The
// second flag is a human's statement that they have read TermsURL and the
// terms allow use on a commercial site; it is false until someone has done
// that, and no code path sets it.
type Source struct {
	Key                   string
	Name                  string
	URL                   string
	Type                  SourceType
	AttributionText       string
	Enabled               bool
	CommercialUseVerified bool
	TermsURL              string
	// Supporting sources corroborate stories but never lead one.
	Supporting bool
}

// Registry is every source ScoutPulse knows how to read.
//
// To turn one on: read its TermsURL, and only if the terms allow use on a
// commercial site, set CommercialUseVerified to true here and deploy. Do not
// add Google News RSS, NewsAPI.org, GNews or The Guardian's free API: their
// free terms rule out commercial use.
var Registry = []Source{
	{
		Key:                   "bbc-sport",
		Name:                  "BBC Sport",
		URL:                   "https://feeds.bbci.co.uk/sport/football/rss.xml",
		Type:                  SourceRSS,
		AttributionText:       "Headline and link from BBC Sport",
		Enabled:               true,
		CommercialUseVerified: false,
		TermsURL:              "https://www.bbc.co.uk/usingthebbc/terms/can-i-use-bbc-content-on-my-website",
	},
	{
		Key:                   "newsdata",
		Name:                  "NewsData.io",
		URL:                   "https://newsdata.io/api/1/latest",
		Type:                  SourceNewsData,
		AttributionText:       "via NewsData.io",
		Enabled:               true,
		CommercialUseVerified: false,
		TermsURL:              "https://newsdata.io/terms-and-condition",
		Supporting:            true,
	},
}

// Active returns the sources that may be fetched.
func Active(registry []Source) []Source {
	var out []Source
	for _, s := range registry {
		if s.Enabled && s.CommercialUseVerified {
			out = append(out, s)
		}
	}
	return out
}

// LogRegistry prints each source with its state and terms link, so whoever
// runs the service can see what is off and where to check before turning it
// on.
func LogRegistry(logger *slog.Logger, registry []Source) {
	for _, s := range registry {
		logger.Info("news source",
			"source", s.Key,
			"enabled", s.Enabled,
			"commercial_use_verified", s.CommercialUseVerified,
			"fetched", s.Enabled && s.CommercialUseVerified,
			"terms_url", s.TermsURL)
	}
}
