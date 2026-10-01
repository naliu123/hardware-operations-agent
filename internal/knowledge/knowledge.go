package knowledge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"hwops/internal/domain"
)

const maxFragmentBytes = 12 * 1024
const maxRepresentationBytes = 4 * 1024
const maxRepresentations = 64

type textChunk struct {
	content            string
	startLine, endLine int
}

func splitFragment(content string, startLine, maxBytes int) []textChunk {
	lines := strings.Split(content, "\n")
	var chunks []textChunk
	var current []string
	currentBytes, chunkStart := 0, startLine
	flush := func(endLine int) {
		text := strings.TrimSpace(strings.Join(current, "\n"))
		if text != "" {
			chunks = append(chunks, textChunk{content: text, startLine: chunkStart, endLine: endLine})
		}
		current, currentBytes = nil, 0
	}
	for offset, line := range lines {
		lineNumber := startLine + offset
		added := len(line)
		if len(current) > 0 {
			added++
		}
		if len(current) > 0 && currentBytes+added > maxBytes {
			flush(lineNumber - 1)
			chunkStart = lineNumber
		}
		for len(line) > maxBytes {
			cut := maxBytes
			for cut > 0 && !utf8.RuneStart(line[cut]) {
				cut--
			}
			if cut == 0 {
				cut = maxBytes
			}
			current = []string{line[:cut]}
			currentBytes = cut
			flush(lineNumber)
			line = line[cut:]
			chunkStart = lineNumber
		}
		if len(current) == 0 {
			chunkStart = lineNumber
		}
		current = append(current, line)
		currentBytes += len(line)
		if len(current) > 1 {
			currentBytes++
		}
	}
	flush(startLine + len(lines) - 1)
	return chunks
}

func NewRevision(in domain.RevisionInput) (domain.Revision, error) {
	in.Content = strings.ReplaceAll(in.Content, "\r\n", "\n")
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Source) == "" ||
		strings.TrimSpace(in.Content) == "" || len(in.Content) > 256*1024 {
		return domain.Revision{}, fmt.Errorf("%w: title, source and text content are required (256 KiB max)", domain.ErrInvalid)
	}
	if in.Applicability.Scope != "GENERAL" && in.Applicability.Scope != "DEVICE" {
		return domain.Revision{}, fmt.Errorf("%w: applicability.scope must be GENERAL or DEVICE", domain.ErrInvalid)
	}
	if in.Applicability.Scope == "GENERAL" && (in.Applicability.Model != "" || in.Applicability.Firmware != "" ||
		in.Applicability.Driver != "" || in.Applicability.HardwareRevision != "" || in.Applicability.PolicyID != "" ||
		in.Applicability.FirmwareRange != nil || in.Applicability.DriverRange != nil) {
		return domain.Revision{}, fmt.Errorf("%w: GENERAL cannot restrict model or versions", domain.ErrInvalid)
	}
	if in.Applicability.Scope == "DEVICE" && in.Applicability.Model == "" {
		return domain.Revision{}, fmt.Errorf("%w: DEVICE requires model", domain.ErrInvalid)
	}
	for _, requirement := range []struct {
		exact  string
		bounds *domain.VersionRange
	}{
		{in.Applicability.Firmware, in.Applicability.FirmwareRange},
		{in.Applicability.Driver, in.Applicability.DriverRange},
	} {
		if requirement.bounds != nil && (requirement.exact != "" || requirement.bounds.Min == "" || requirement.bounds.Max == "") {
			return domain.Revision{}, fmt.Errorf("%w: a closed version range requires min/max and cannot coexist with an exact version", domain.ErrInvalid)
		}
	}
	r := domain.Revision{
		SchemaVersion: 1, ID: rand.Text(), DocumentID: rand.Text(),
		Title: in.Title, Source: in.Source, Content: in.Content,
		Applicability: in.Applicability, Status: "DRAFT", CreatedAt: time.Now().UTC(),
	}
	if len(in.Fragments) > 0 {
		fragments, err := validateEnrichedFragments(r.ID, in.Content, in.Fragments)
		if err != nil {
			return domain.Revision{}, err
		}
		r.SchemaVersion = 2
		r.Fragments = fragments
		return r, nil
	}
	section, start := "正文", 1
	var body []string
	flush := func(end int) {
		content := strings.TrimSpace(strings.Join(body, "\n"))
		if content != "" {
			chunks := splitFragment(content, start, maxFragmentBytes)
			for i, chunk := range chunks {
				chunkSection := section
				if len(chunks) > 1 {
					chunkSection = fmt.Sprintf("%s（第%d部分）", section, i+1)
				}
				r.Fragments = append(r.Fragments, domain.Fragment{
					ID: rand.Text(), RevisionID: r.ID, Section: chunkSection,
					StartLine: chunk.startLine, EndLine: chunk.endLine, Content: chunk.content,
					ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(chunk.content))),
				})
			}
		}
		body = nil
	}
	lines := strings.Split(in.Content, "\n")
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "#") && strings.Contains(line, " ") {
			flush(i)
			section = strings.TrimSpace(strings.TrimLeft(line, "#"))
			start = i + 2
		} else {
			body = append(body, line)
		}
	}
	flush(len(lines))
	if len(r.Fragments) == 0 {
		return domain.Revision{}, fmt.Errorf("%w: no text fragments", domain.ErrInvalid)
	}
	return r, nil
}

func validateEnrichedFragments(revisionID, content string, inputs []domain.FragmentInput) ([]domain.Fragment, error) {
	if len(inputs) > 4096 {
		return nil, fmt.Errorf("%w: too many enriched fragments", domain.ErrInvalid)
	}
	lines := strings.Split(content, "\n")
	covered := make([]bool, len(lines))
	fragments := make([]domain.Fragment, 0, len(inputs))
	previousEnd := 0
	for _, input := range inputs {
		section := strings.TrimSpace(input.Section)
		if section == "" || len(section) > 512 || input.StartLine < 1 ||
			input.EndLine < input.StartLine || input.EndLine > len(lines) ||
			input.StartLine <= previousEnd {
			return nil, fmt.Errorf("%w: invalid enriched fragment range or section", domain.ErrInvalid)
		}
		expected := strings.TrimSpace(strings.Join(lines[input.StartLine-1:input.EndLine], "\n"))
		if expected == "" || input.Content != expected || len(input.Content) > maxFragmentBytes {
			return nil, fmt.Errorf("%w: enriched fragment must match a bounded original line range", domain.ErrInvalid)
		}
		representations, err := validateRepresentations(
			input.Content, input.StartLine, input.EndLine, input.Representations,
		)
		if err != nil {
			return nil, err
		}
		for line := input.StartLine - 1; line < input.EndLine; line++ {
			covered[line] = true
		}
		fragments = append(fragments, domain.Fragment{
			ID: rand.Text(), RevisionID: revisionID, Section: section,
			StartLine: input.StartLine, EndLine: input.EndLine, Content: input.Content,
			ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(input.Content))),
			Representations: representations,
		})
		previousEnd = input.EndLine
	}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || covered[i] || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "来源：") || strings.HasPrefix(line, "目录：") ||
			strings.HasPrefix(line, "更新时间：") {
			continue
		}
		return nil, fmt.Errorf("%w: enriched fragments omit source line %d", domain.ErrInvalid, i+1)
	}
	return fragments, nil
}

func validateRepresentations(
	content string,
	fragmentStart, fragmentEnd int,
	inputs []domain.RetrievalRepresentation,
) ([]domain.RetrievalRepresentation, error) {
	if len(inputs) < 2 || len(inputs) > maxRepresentations {
		return nil, fmt.Errorf("%w: enriched fragment requires 2..%d retrieval representations", domain.ErrInvalid, maxRepresentations)
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	out := make([]domain.RetrievalRepresentation, 0, len(inputs))
	for _, input := range inputs {
		input.Kind = strings.ToUpper(strings.TrimSpace(input.Kind))
		input.Text = strings.TrimSpace(input.Text)
		if (input.Kind != "QUESTION" && input.Kind != "TABLE" &&
			input.Kind != "TABLE_TEXT" && input.Kind != "TABLE_SUMMARY" &&
			input.Kind != "IMAGE" && input.Kind != "PASSAGE") ||
			input.Text == "" || len(input.Text) > maxRepresentationBytes ||
			seen[fmt.Sprintf("%s\x00%s\x00%d\x00%d", input.Kind, input.Text, input.StartLine, input.EndLine)] {
			return nil, fmt.Errorf("%w: invalid or duplicate retrieval representation", domain.ErrInvalid)
		}
		if (input.StartLine == 0) != (input.EndLine == 0) ||
			input.StartLine < 0 || input.EndLine < 0 {
			return nil, fmt.Errorf("%w: retrieval representation requires a complete source range", domain.ErrInvalid)
		}
		if input.StartLine > 0 {
			if input.Kind != "TABLE" && input.Kind != "TABLE_TEXT" &&
				input.Kind != "TABLE_SUMMARY" && input.Kind != "IMAGE" {
				return nil, fmt.Errorf("%w: only asset representations may have a source range", domain.ErrInvalid)
			}
			if input.StartLine < fragmentStart || input.EndLine < input.StartLine || input.EndLine > fragmentEnd {
				return nil, fmt.Errorf("%w: asset representation source range is outside its fragment", domain.ErrInvalid)
			}
			sourceLines := strings.Split(content, "\n")
			from := input.StartLine - fragmentStart
			to := input.EndLine - fragmentStart + 1
			source := strings.Join(sourceLines[from:to], "\n")
			switch {
			case input.Kind == "IMAGE" && (!strings.Contains(source, "![") || !strings.Contains(source, "](")):
				return nil, fmt.Errorf("%w: image representation source range has no image", domain.ErrInvalid)
			case strings.HasPrefix(input.Kind, "TABLE") && !containsMarkdownTable(source):
				return nil, fmt.Errorf("%w: table representation source range has no table", domain.ErrInvalid)
			}
		}
		seen[fmt.Sprintf("%s\x00%s\x00%d\x00%d", input.Kind, input.Text, input.StartLine, input.EndLine)] = true
		counts[input.Kind]++
		out = append(out, input)
	}
	if counts["QUESTION"] < 2 || counts["QUESTION"] > 6 {
		return nil, fmt.Errorf("%w: each enriched fragment requires 2..6 questions", domain.ErrInvalid)
	}
	if containsMarkdownTable(content) &&
		counts["TABLE"]+counts["TABLE_TEXT"]+counts["TABLE_SUMMARY"] == 0 {
		return nil, fmt.Errorf("%w: table fragment requires a table description", domain.ErrInvalid)
	}
	if containsMarkdownImage(content) && counts["IMAGE"] == 0 {
		return nil, fmt.Errorf("%w: image fragment requires an image description", domain.ErrInvalid)
	}
	return out, nil
}

// RetrievalContext keeps the immutable source text and inserts generated asset
// descriptions immediately after the source lines they describe. Questions and
// embedding-only passages are deliberately excluded from answer context.
func RetrievalContext(fragment domain.Fragment) string {
	labels := map[string]string{
		"TABLE":         "表格说明",
		"TABLE_TEXT":    "表格说明",
		"TABLE_SUMMARY": "表格摘要",
		"IMAGE":         "图片说明",
	}
	positioned := map[int][]string{}
	var trailing []string
	for _, representation := range fragment.Representations {
		label, ok := labels[representation.Kind]
		if !ok {
			continue
		}
		text := fmt.Sprintf("[%s] %s", label, representation.Text)
		if representation.EndLine > 0 {
			positioned[representation.EndLine] = append(positioned[representation.EndLine], text)
		} else {
			trailing = append(trailing, text)
		}
	}
	lines := strings.Split(fragment.Content, "\n")
	output := make([]string, 0, len(lines)+len(fragment.Representations))
	for offset, line := range lines {
		output = append(output, line)
		output = append(output, positioned[fragment.StartLine+offset]...)
	}
	if len(trailing) > 0 {
		output = append(output, trailing...)
	}
	return strings.Join(output, "\n")
}

func containsMarkdownTable(content string) bool {
	fenced := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") && strings.Count(line, "|") >= 3 {
			return true
		}
	}
	return false
}

func containsMarkdownImage(content string) bool {
	fenced := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if !fenced && strings.Contains(line, "![") && strings.Contains(line, "](") {
			return true
		}
	}
	return false
}

// LocalRetriever is a lexical baseline. Hybrid recall is delivered by QA-03.
// Applicability filtering and the final context budget are applied by the graph.
type LocalRetriever struct {
	Store domain.Repository
}

var _ retriever.Retriever = (*LocalRetriever)(nil)

func terms(text string) map[string]bool {
	set := map[string]bool{}
	var run []rune
	flush := func() {
		if len(run) > 0 {
			set[string(run)] = true
		}
		for i := 0; i+1 < len(run); i++ {
			set[string(run[i:i+2])] = true
		}
		run = nil
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			run = append(run, r)
		} else {
			flush()
		}
	}
	flush()
	return set
}

func (r *LocalRetriever) Retrieve(ctx context.Context, query string, _ ...retriever.Option) ([]*schema.Document, error) {
	revisions, err := r.Store.ListPublished(ctx)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		doc   *schema.Document
		score int
	}
	var matches []candidate
	q := terms(query)
	for _, revision := range revisions {
		for _, fragment := range revision.Fragments {
			score := 0
			for term := range terms(revision.Title + " " + fragment.Section + " " + fragment.Content) {
				if q[term] {
					score++
				}
			}
			if score > 0 {
				matches = append(matches, candidate{doc: &schema.Document{
					ID: fragment.ID, Content: fragment.Content,
					MetaData: map[string]any{"revision_id": revision.ID},
				}, score: score})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].doc.ID < matches[j].doc.ID
		}
		return matches[i].score > matches[j].score
	})
	var docs []*schema.Document
	for _, m := range matches {
		docs = append(docs, m.doc)
	}
	return docs, nil
}
