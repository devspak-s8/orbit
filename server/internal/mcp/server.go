// Package mcp implements a Model Context Protocol server endpoint: a
// JSON-RPC 2.0 interface that lets external AI assistants (Claude, Cursor,
// ChatGPT) operate ORBIT tasks, records, pages, and workflows directly.
// Bearer tokens are the auth layer; every tool call is audit-attributed.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/agents"
	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/automation"
	"github.com/orbit-org/orbit/server/internal/data"
	"github.com/orbit-org/orbit/server/internal/docs"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/work"
)

// Server exposes ORBIT operations as MCP tools.
type Server struct {
	pool   *pgxpool.Pool
	ident  *identity.Service
	agents *agents.Service
	work   *work.Service
	data   *data.Service
	docs   *docs.Service
	auto   *automation.Service
}

func New(pool *pgxpool.Pool, ident *identity.Service, ag *agents.Service, w *work.Service, d *data.Service, dc *docs.Service, au *automation.Service) *Server {
	return &Server{pool: pool, ident: ident, agents: ag, work: w, data: d, docs: dc, auto: au}
}

// --- JSON-RPC 2.0 ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// HandleMCP is the single endpoint. Auth: Bearer token.
func (s *Server) HandleMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := s.identify(r.Context(), r)
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "Bearer token required")
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		s.writeRPC(w, nil, &rpcError{Code: -32700, Message: "parse error"})
		return
	}

	switch req.Method {
	case "initialize":
		s.writeRPC(w, req.ID, map[string]any{
			"protocolVersion": "1.0",
			"serverInfo":      map[string]any{"name": "ORBIT", "version": "1.0.0"},
			"capabilities":    map[string]any{"tools": true},
		})
	case "tools/list":
		s.writeRPC(w, req.ID, map[string]any{"tools": toolDefs})
	case "tools/call":
		s.handleToolCall(w, r, req, user)
	default:
		s.writeRPC(w, req.ID, &rpcError{Code: -32601, Message: "method not found: " + req.Method})
	}
}

func (s *Server) identify(ctx context.Context, r *http.Request) (identity.User, bool) {
	const prefix = "Bearer "
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, prefix) {
		return s.ident.UserByBearerToken(ctx, strings.TrimPrefix(h, prefix))
	}
	return identity.User{}, false
}

func (s *Server) writeRPC(w http.ResponseWriter, id json.RawMessage, result any) {
	apijson.WriteJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	apijson.WriteJSON(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func (s *Server) handleToolCall(w http.ResponseWriter, r *http.Request, req rpcRequest, user identity.User) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeRPCError(w, req.ID, -32602, "invalid params")
		return
	}

	var args map[string]any
	_ = json.Unmarshal(params.Arguments, &args)

	result, err := s.executeTool(r.Context(), user, params.Name, args)
	if err != nil {
		slog.Error("mcp tool call", "tool", params.Name, "error", err, "user", user.ID)
		s.writeRPCError(w, req.ID, -32000, err.Error())
		return
	}
	s.writeRPC(w, req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": result}}})
}

// --- tool definitions ---

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var toolDefs = []toolDef{
	{
		Name:        "orbit_list_projects",
		Description: "List all projects in a workspace. Returns id, name, and task counts.",
		InputSchema: propSchema("workspaceId"),
	},
	{
		Name:        "orbit_create_task",
		Description: "Create a task in a project. Returns the task with id and status.",
		InputSchema: propSchema("workspaceId", "projectId", "title", "assigneeId?", "priority?", "dueDate?"),
	},
	{
		Name:        "orbit_list_tasks",
		Description: "List tasks in a project, optionally filtered by status.",
		InputSchema: propSchema("workspaceId", "projectId", "status?"),
	},
	{
		Name:        "orbit_complete_task",
		Description: "Mark a task as done. Returns the completed task.",
		InputSchema: propSchema("workspaceId", "taskId"),
	},
	{
		Name:        "orbit_create_record",
		Description: "Create a record in a data table. 'values' is a JSON object keyed by column id.",
		InputSchema: propSchema("workspaceId", "tableId", "values"),
	},
	{
		Name:        "orbit_list_records",
		Description: "List records in a data table.",
		InputSchema: propSchema("workspaceId", "tableId"),
	},
	{
		Name:        "orbit_run_workflow",
		Description: "Trigger a manual run of a validated workflow. Returns the run id.",
		InputSchema: propSchema("workspaceId", "workflowId"),
	},
	{
		Name:        "orbit_list_workflows",
		Description: "List workflows in a workspace with their status and trigger type.",
		InputSchema: propSchema("workspaceId"),
	},
	{
		Name:        "orbit_create_page",
		Description: "Create a documentation page on a project.",
		InputSchema: propSchema("workspaceId", "projectId", "title", "body"),
	},
	{
		Name:        "orbit_list_pages",
		Description: "List documentation pages, optionally filtered by project.",
		InputSchema: propSchema("workspaceId", "projectId?"),
	},
}

func propSchema(required ...string) map[string]any {
	props := map[string]any{
		"workspaceId": map[string]any{"type": "string", "description": "The workspace ID"},
		"projectId":   map[string]any{"type": "string", "description": "The project ID"},
		"tableId":     map[string]any{"type": "string", "description": "The table ID"},
		"taskId":      map[string]any{"type": "string", "description": "The task ID"},
		"workflowId":  map[string]any{"type": "string", "description": "The workflow ID"},
		"title":       map[string]any{"type": "string", "description": "Title text"},
		"body":        map[string]any{"type": "string", "description": "Body text (markdown for pages)"},
		"values":      map[string]any{"type": "object", "description": "JSON object keyed by column id"},
		"status":      map[string]any{"type": "string", "enum": []string{"todo", "in_progress", "done"}},
		"assigneeId":  map[string]any{"type": "string", "description": "User ID of the assignee"},
		"priority":    map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "urgent"}},
		"dueDate":     map[string]any{"type": "string", "description": "YYYY-MM-DD"},
	}
	req := []string{}
	for _, r := range required {
		if !strings.HasSuffix(r, "?") {
			req = append(req, r)
		}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   req,
	}
}

// --- tool execution ---

func (s *Server) executeTool(ctx context.Context, user identity.User, name string, args map[string]any) (string, error) {
	str := func(key string) string {
		v, _ := args[key].(string)
		return v
	}
	ws := str("workspaceId")

	switch name {
	case "orbit_list_projects":
		items, err := s.work.ListProjects(ctx, user.ID, ws)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(items)
		return string(b), nil

	case "orbit_create_task":
		patch := work.TaskPatch{
			AssigneeID: optStr(args, "assigneeId"),
			DueDate:    optStr(args, "dueDate"),
		}
		if p := str("priority"); p != "" {
			patch.Priority = &p
		}
		task, err := s.work.CreateTask(ctx, user.ID, ws, str("projectId"), str("title"), patch)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(task)
		return string(b), nil

	case "orbit_list_tasks":
		items, err := s.work.ListTasks(ctx, user.ID, ws, str("projectId"), str("status"))
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(items)
		return string(b), nil

	case "orbit_complete_task":
		task, err := s.work.SetTaskStatus(ctx, user.ID, ws, str("taskId"), "done")
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(task)
		return string(b), nil

	case "orbit_create_record":
		values, _ := args["values"].(map[string]any)
		rec, err := s.data.CreateRecord(ctx, user.ID, ws, str("tableId"), values, "mcp")
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(rec)
		return string(b), nil

	case "orbit_list_records":
		items, err := s.data.ListRecords(ctx, user.ID, ws, str("tableId"), "", "")
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(items)
		return string(b), nil

	case "orbit_run_workflow":
		run, err := s.auto.StartRun(ctx, user.ID, ws, str("workflowId"))
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(map[string]any{"runId": run.ID, "status": run.Status})
		return string(b), nil

	case "orbit_list_workflows":
		items, err := s.auto.List(ctx, user.ID, ws)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(items)
		return string(b), nil

	case "orbit_create_page":
		page, err := s.docs.CreatePage(ctx, user.ID, ws, str("projectId"), str("title"), str("body"))
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(page)
		return string(b), nil

	case "orbit_list_pages":
		items, err := s.docs.ListPages(ctx, user.ID, ws, str("projectId"))
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(items)
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func optStr(args map[string]any, key string) *string {
	if v, ok := args[key].(string); ok && v != "" {
		return &v
	}
	return nil
}
