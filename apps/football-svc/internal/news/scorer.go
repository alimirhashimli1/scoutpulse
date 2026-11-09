package news

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// Candidate is a story put in front of a scorer.
type Candidate struct {
	// Key ties a verdict back to its story within one batch.
	Key         string
	Headline    string
	Description string
	Outlets     int
	// SupportingOnly is true when every report so far is supporting
	// coverage, i.e. there is no first-hand feed behind it yet.
	SupportingOnly bool
	FirstSeen      time.Time
}

// AiScorer scores a batch of stories with a language model. It returns
// verdicts keyed by Candidate.Key; a candidate missing from the map was not
// scored, and the caller falls back to rules for it.
type AiScorer interface {
	ScoreBatch(ctx context.Context, batch []Candidate) (map[string]Verdict, error)
}

// RawVerdict is a verdict as a model wrote it, before validation.
type RawVerdict struct {
	ID       string      `json:"id"`
	Score    json.Number `json:"score"`
	Category string      `json:"category"`
	Reason   string      `json:"reason"`
	Summary  string      `json:"summary"`
}

const (
	maxSummaryWords = 40
	maxReasonRunes  = 240
)

var linkPattern = regexp.MustCompile(`(?i)(https?://|www\.|<[a-z/])`)

// ValidateVerdict checks a model's verdict: an integer score from 1 to 10,
// one of the six categories, a reason, and a plain-text summary of at most 40
// words. Anything else is rejected rather than repaired, because a model that
// got the shape wrong may have got the judgement wrong too.
func ValidateVerdict(raw RawVerdict) (Verdict, error) {
	f, err := raw.Score.Float64()
	if err != nil {
		return Verdict{}, errors.New("score is not a number")
	}
	if f != math.Trunc(f) || f < 1 || f > 10 {
		return Verdict{}, fmt.Errorf("score %v is not an integer from 1 to 10", f)
	}
	cat, ok := ParseCategory(raw.Category)
	if !ok {
		return Verdict{}, fmt.Errorf("unknown category %q", raw.Category)
	}
	// Checked on the raw text: cleaning would strip the very markup this
	// is looking for.
	if linkPattern.MatchString(raw.Summary) || linkPattern.MatchString(raw.Reason) {
		return Verdict{}, errors.New("summary or reason contains a link or markup")
	}
	reason := CleanText(raw.Reason)
	if reason == "" {
		return Verdict{}, errors.New("reason is empty")
	}
	summary := CleanText(raw.Summary)
	if summary == "" {
		return Verdict{}, errors.New("summary is empty")
	}
	if n := len(strings.Fields(summary)); n > maxSummaryWords {
		return Verdict{}, fmt.Errorf("summary has %d words, the limit is %d", n, maxSummaryWords)
	}
	return Verdict{
		Score: int(f), Category: cat, Reason: clip(reason, maxReasonRunes),
		Summary: summary, Scorer: "ai",
	}, nil
}

// Adjust applies the weights every scorer shares: coverage by several
// outlets adds a point, and a story only supporting coverage has reported
// loses one, since that coverage is half a day late and second-hand.
func Adjust(v Verdict, c Candidate) Verdict {
	if c.Outlets >= 2 {
		v.Score++
	}
	if c.Outlets >= 4 {
		v.Score++
	}
	if c.SupportingOnly {
		v.Score--
	}
	v.Score = min(10, max(1, v.Score))
	return v
}

// --- rules -------------------------------------------------------------------

type rule struct {
	pattern *regexp.Regexp
	weight  int
	why     string
}

func words(parts ...string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b(` + strings.Join(parts, "|") + `)\b`)
}

// Weights from a base of 5. Only the strongest positive rule counts in full,
// with a point more if a second, different one also matches -- so one strong
// rule makes 7, one major rule 9, and stacking cannot inflate a routine story.
// Every negative rule counts.
var rules = []rule{
	{words(`sack`, `sacked`, `sacks`, `sacking`, `fired`, `dismissed`, `parts? ways? with (manager|head coach|boss)`), 4, "manager sacking"},
	{words(`appointed`, `appoints`, `named (new )?(manager|head coach|boss)`, `new (manager|head coach|boss)`), 4, "manager appointment"},
	{words(`(world|club|british|league)[- ]record`, `record (fee|signing|transfer|deal)`), 4, "record transfer"},
	{words(`career[- ]threatening`, `acl`, `cruciate`, `rest of the season`, `season[- ]ending`, `out for (the )?season`), 4, "serious injury"},
	{words(`points? deduct(ion|ed)`, `transfer ban`, `banned`, `expelled`, `relegated as punishment`, `charged by`, `sanction(s|ed)?`), 4, "major sanction"},
	{words(`completes?`, `signs?`, `signed`, `joins`, `joined`, `confirmed`, `confirms`, `agrees? (a )?deal`, `seals?`), 2, "confirmed move"},
	{words(`contract`, `extension`, `new deal`, `release clause`), 2, "contract news"},
	{words(`injur(y|ed|ies)`, `ruled out`, `sidelined`, `surgery`, `hamstring`, `out for`), 2, "injury"},
	{words(`wins?`, `won`, `beat`, `beats`, `thrash(es|ed)?`, `final`, `title`, `champions`, `knocked out`, `eliminated`, `\d+-\d+`), 2, "result"},
	{words(`wonderkid`, `teenager`, `teen`, `academy`, `youngster`, `prodigy`, `debut`, `breakout`, `breakthrough`, `hat-trick`, `(1[5-9]|2[01])-year-old`, `u-?(17|19|21|23)`), 2, "young talent"},
	{words(`rumou?rs?`, `linked`, `eye(s|ing)?`, `considering`, `interested`, `could`, `might`, `may`, `set to`, `in talks`, `gossip`, `target(s|ing)?`), -2, "unconfirmed"},
	{words(`preview`, `predictions?`, `predicted`, `odds`, `betting`, `bet`, `tips`, `ranked`, `ranking`, `top \d+`, `best xi`, `opinion`, `column`, `quiz`, `podcast`, `live`, `watch`, `explained`), -3, "preview, opinion or listicle"},
}

var categoryRules = []struct {
	cat     Category
	pattern *regexp.Regexp
}{
	{CategoryManagers, words(`manager`, `head coach`, `boss`, `sack`, `sacked`, `sacks`, `appointed`, `appoints`, `coach`)},
	{CategoryInjuries, words(`injur(y|ed|ies)`, `ruled out`, `sidelined`, `surgery`, `hamstring`, `acl`, `cruciate`, `knee`, `ankle`)},
	{CategoryYoungTalents, words(`wonderkid`, `teenager`, `teen`, `academy`, `youngster`, `prodigy`, `debut`, `(1[5-9]|2[01])-year-old`, `u-?(17|19|21|23)`)},
	{CategoryTransfers, words(`transfer`, `signs?`, `signed`, `signing`, `joins`, `joined`, `loan`, `fee`, `deal`, `bid`, `contract`, `move`)},
	{CategoryResults, words(`wins?`, `won`, `beat`, `beats`, `draw`, `drew`, `defeat`, `thrash(es|ed)?`, `final`, `title`, `champions`, `\d+-\d+`)},
}

// RuleScore scores a story from its headline alone: keywords, the number of
// outlets, and how recent it is. It writes no summary -- it cannot write
// original prose, and the feed's wording is not ours to republish.
func RuleScore(c Candidate, now time.Time) Verdict {
	var why []string
	text := c.Headline
	best, positives, negatives := 0, 0, 0
	for _, r := range rules {
		if !r.pattern.MatchString(text) {
			continue
		}
		why = append(why, r.why)
		if r.weight > 0 {
			positives++
			best = max(best, r.weight)
		} else {
			negatives += r.weight
		}
	}
	score := 5 + best + negatives
	if positives >= 2 {
		score++
	}
	if strings.HasSuffix(strings.TrimSpace(c.Headline), "?") {
		score -= 2
		why = append(why, "question headline")
	}
	if !c.FirstSeen.IsZero() && now.Sub(c.FirstSeen) > 24*time.Hour {
		score--
		why = append(why, "over a day old")
	}

	cat := CategoryOther
	for _, cr := range categoryRules {
		if cr.pattern.MatchString(text) {
			cat = cr.cat
			break
		}
	}

	reason := "rules: no strong signal"
	if len(why) > 0 {
		reason = "rules: " + strings.Join(why, ", ")
	}
	if c.Outlets >= 2 {
		reason += fmt.Sprintf("; %d outlets", c.Outlets)
	}
	return Adjust(Verdict{
		Score: min(10, max(1, score)), Category: cat, Reason: reason, Scorer: "rules",
	}, c)
}
