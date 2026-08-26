package mcp

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const toolSearchKnowledgeBase = "search_knowledge_base"

// searchRequest flattens the API's nested scope object into four sibling lists.
//
// The flattening is deliberate. On the wire, an absent scope searches the whole
// project and a present-but-empty scope searches nothing — a distinction that
// is easy for a model to trip over and never what an agent wants. With the
// lists flat, "no lists given" can only mean the whole project.
type searchRequest struct {
	Query       string   `json:"query" jsonschema:"the question to answer, in natural language; this is a semantic search, so write a question rather than keywords"`
	MaxResults  int      `json:"max_results,omitempty" jsonschema:"maximum passages to return, 1 to 50; defaults to 10"`
	AssetIDs    []string `json:"asset_ids,omitempty" jsonschema:"restrict the search to these assets; omit every list to search the whole project"`
	LabelIDs    []string `json:"label_ids,omitempty" jsonschema:"restrict the search to assets carrying these labels"`
	ViewIDs     []string `json:"view_ids,omitempty" jsonschema:"restrict the search to the assets in these views"`
	ConceptIDs  []string `json:"concept_ids,omitempty" jsonschema:"restrict the search to the assets behind these concepts"`
	IndexedOnly bool     `json:"indexed_only,omitempty" jsonschema:"skip the slower pass that reads documents still being ingested; set this when speed matters more than covering a just-uploaded file"`
}

func registerSearchTools(s *mcpsdk.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolSearchKnowledgeBase,
		desc: "Search the project's documents and return the passages that answer a question. " +
			"Use this whenever the user asks about their own files, uploads, or project content, " +
			"rather than guessing from memory. " +
			"The search is semantic, so phrase the query as a natural question; keywords work poorly. " +
			"Searches the whole project unless you pass asset_ids, label_ids, view_ids, or concept_ids. " +
			"Each result carries an asset and its matching passages, best first, with a relevance_score from 0 to 1 where higher is a stronger match. " +
			"Quote the passages rather than paraphrasing them, and cite the asset title. " +
			"Requires a paid plan.",
	}, searchKnowledgeBase)
}

func searchKnowledgeBase(ctx context.Context, c *chief.Client, req searchRequest) (*chief.SearchResponse, string, error) {
	search := &chief.SearchRequest{
		Query:      req.Query,
		MaxResults: req.MaxResults,
		Scope:      scopeFromSearchRequest(req),
	}
	if req.IndexedOnly {
		search.Include = []chief.SearchInclude{chief.SearchIncludeUnstructured}
	}

	resp, err := c.Search.Search(ctx, search)
	if err != nil {
		return nil, "", fmt.Errorf("search %q: %w", req.Query, err)
	}

	return resp, searchSummary(resp), nil
}

// scopeFromSearchRequest returns nil when no list was given, which is what the
// API reads as "the whole project". Sending an empty scope object instead would
// search nothing.
func scopeFromSearchRequest(req searchRequest) *chief.SearchScope {
	if len(req.AssetIDs) == 0 && len(req.LabelIDs) == 0 && len(req.ViewIDs) == 0 && len(req.ConceptIDs) == 0 {
		return nil
	}
	return &chief.SearchScope{
		AssetIDs:   req.AssetIDs,
		LabelIDs:   req.LabelIDs,
		ViewIDs:    req.ViewIDs,
		ConceptIDs: req.ConceptIDs,
	}
}

// searchSummary is the one-line text result. It reports a partial response so
// the model can say the answer may be incomplete instead of presenting it as
// the whole picture.
func searchSummary(resp *chief.SearchResponse) string {
	if len(resp.Results) == 0 {
		return fmt.Sprintf("no matches for %q", resp.Query)
	}

	summary := fmt.Sprintf("found %d passage(s) across %d document(s) in %dms",
		resp.TotalResults, len(resp.Results), resp.DurationMS)
	if resp.Partial {
		summary += "; the search timed out before finishing, so results may be incomplete"
	}
	return summary
}
