package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolCreateChat    = "create_chat"
	toolListChats     = "list_chats"
	toolGetChat       = "get_chat"
	toolUpdateChat    = "update_chat"
	toolDeleteChat    = "delete_chat"
	toolSendMessage   = "send_message"
	toolListMessages  = "list_messages"
	toolGetMessage    = "get_message"
	toolDeleteMessage = "delete_message"
)

const defaultChatResponseTimeout = 360 * time.Second

type createChatRequest struct {
	Chat            chief.CreateChatRequest `json:"chat" jsonschema:"prompt is required. intelligence is one of \"auto\", \"fast\", \"expert\", \"research\" and empty defaults to \"auto\". provider is one of \"automatic\", \"anthropic\", \"openai\", \"google\". An empty scope sees the whole project"`
	WaitForResponse bool                    `json:"wait_for_response,omitempty" jsonschema:"block until the turn finishes and the response is ready before returning"`
	TimeoutSeconds  int                     `json:"timeout_seconds,omitempty" jsonschema:"seconds to wait when wait_for_response is set; defaults to 360"`
}

type sendMessageRequest struct {
	ChatID          string                   `json:"chat_id" jsonschema:"the chat to append a turn to"`
	Message         chief.SendMessageRequest `json:"message" jsonschema:"prompt is required. The tuning fields match create_chat"`
	WaitForResponse bool                     `json:"wait_for_response,omitempty" jsonschema:"block until the turn finishes and the response is ready before returning"`
	TimeoutSeconds  int                      `json:"timeout_seconds,omitempty" jsonschema:"seconds to wait when wait_for_response is set; defaults to 360"`
}

type listChatsRequest struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of chats to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this chat ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this chat ID"`
}

type chatIDRequest struct {
	ChatID string `json:"chat_id" jsonschema:"the chat ID"`
}

type updateChatRequest struct {
	ChatID string                  `json:"chat_id" jsonschema:"the chat to rename"`
	Chat   chief.UpdateChatRequest `json:"chat" jsonschema:"title is the only mutable field and must not be empty"`
}

type messageRequest struct {
	ChatID    string `json:"chat_id" jsonschema:"the chat containing the message"`
	MessageID string `json:"message_id" jsonschema:"the message ID"`
}

// createChatResponse and sendMessageResponse carry the async accept fields plus
// the resolved Response, which stays empty unless wait_for_response was set.
type createChatResponse struct {
	ChatID    string    `json:"chat_id"`
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response,omitempty"`
}

type sendMessageResponse struct {
	MessageID string    `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response,omitempty"`
}

type deleteChatResponse struct {
	Deleted bool `json:"deleted"`
}

type deleteMessageResponse struct {
	Deleted bool `json:"deleted"`
}

func registerChatTools(s *mcp.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolCreateChat,
		desc: "Create a chat with its first turn. intelligence picks a mode preset and provider biases vendor selection within it. Turns run asynchronously; set wait_for_response to block until the answer is ready, otherwise poll get_message with the returned message_id.",
	}, createChat)
	addTool(s, c, toolMeta{
		name: toolListChats,
		desc: "List the caller's chats in the project, cursor-paginated. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listChats)
	addTool(s, c, toolMeta{
		name: toolGetChat,
		desc: "Get a chat's metadata by ID. modified_at is null until the chat's first turn completes.",
	}, getChat)
	addTool(s, c, toolMeta{
		name: toolUpdateChat,
		desc: "Rename a chat. title is the only mutable field and must not be empty.",
	}, updateChat)
	addTool(s, c, toolMeta{
		name: toolDeleteChat,
		desc: "Delete a chat and its messages permanently.",
	}, deleteChat)
	addTool(s, c, toolMeta{
		name: toolSendMessage,
		desc: "Append a turn to an existing chat. The tuning fields match create_chat. Turns run asynchronously; set wait_for_response to block until the answer is ready, otherwise poll get_message with the returned message_id.",
	}, sendMessage)
	addTool(s, c, toolMeta{
		name: toolListMessages,
		desc: "List metadata for every message in a chat. Message content is fetched separately via get_message.",
	}, listMessages)
	addTool(s, c, toolMeta{
		name: toolGetMessage,
		desc: "Get a single message by ID, including its prompt and response. Both stay empty until the async turn finishes, so poll until the response is present.",
	}, getMessage)
	addTool(s, c, toolMeta{
		name: toolDeleteMessage,
		desc: "Delete a single message from a chat permanently.",
	}, deleteMessage)
}

func createChat(ctx context.Context, c *chief.Client, req createChatRequest) (*createChatResponse, string, error) {
	created, err := c.Chats.Create(ctx, &req.Chat)
	if err != nil {
		return nil, "", fmt.Errorf("create chat: %w", err)
	}

	out := &createChatResponse{ChatID: created.ChatID, MessageID: created.MessageID, CreatedAt: created.CreatedAt}
	summary := fmt.Sprintf("created chat %s (message %s)", created.ChatID, created.MessageID)
	if req.WaitForResponse {
		timeout := defaultChatResponseTimeout
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}
		msg, err := c.Chats.WaitForResponse(ctx, created.ChatID, created.MessageID, timeout)
		if err != nil {
			return nil, "", fmt.Errorf("wait for chat %s message %s: %w", created.ChatID, created.MessageID, err)
		}
		out.Response = msg.Response
		summary = fmt.Sprintf("created chat %s with response (message %s)", created.ChatID, created.MessageID)
	}
	return out, summary, nil
}

func listChats(ctx context.Context, c *chief.Client, req listChatsRequest) (*chief.ChatPage, string, error) {
	var opts []chief.ListOption
	if req.Limit > 0 {
		opts = append(opts, chief.WithLimit(req.Limit))
	}
	if req.AfterID != "" {
		opts = append(opts, chief.WithAfterID(req.AfterID))
	}
	if req.BeforeID != "" {
		opts = append(opts, chief.WithBeforeID(req.BeforeID))
	}

	page, err := c.Chats.List(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("list chats: %w", err)
	}
	return page, fmt.Sprintf("%d chat(s) returned (has_more %t)", len(page.Data), page.HasMore), nil
}

func getChat(ctx context.Context, c *chief.Client, req chatIDRequest) (*chief.ChatResponse, string, error) {
	chat, err := c.Chats.Get(ctx, req.ChatID)
	if err != nil {
		return nil, "", fmt.Errorf("get chat %q: %w", req.ChatID, err)
	}
	return chat, fmt.Sprintf("chat %s", chat.ChatID), nil
}

func updateChat(ctx context.Context, c *chief.Client, req updateChatRequest) (*chief.ChatResponse, string, error) {
	chat, err := c.Chats.Update(ctx, req.ChatID, &req.Chat)
	if err != nil {
		return nil, "", fmt.Errorf("update chat %q: %w", req.ChatID, err)
	}
	return chat, fmt.Sprintf("updated chat %s", chat.ChatID), nil
}

func deleteChat(ctx context.Context, c *chief.Client, req chatIDRequest) (deleteChatResponse, string, error) {
	if err := c.Chats.Delete(ctx, req.ChatID); err != nil {
		return deleteChatResponse{}, "", fmt.Errorf("delete chat %q: %w", req.ChatID, err)
	}
	return deleteChatResponse{Deleted: true}, fmt.Sprintf("deleted chat %s", req.ChatID), nil
}

func sendMessage(ctx context.Context, c *chief.Client, req sendMessageRequest) (*sendMessageResponse, string, error) {
	sent, err := c.Chats.SendMessage(ctx, req.ChatID, &req.Message)
	if err != nil {
		return nil, "", fmt.Errorf("send message to chat %q: %w", req.ChatID, err)
	}

	out := &sendMessageResponse{MessageID: sent.MessageID, CreatedAt: sent.CreatedAt}
	summary := fmt.Sprintf("sent message %s to chat %s", sent.MessageID, req.ChatID)
	if req.WaitForResponse {
		timeout := defaultChatResponseTimeout
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}
		msg, err := c.Chats.WaitForResponse(ctx, req.ChatID, sent.MessageID, timeout)
		if err != nil {
			return nil, "", fmt.Errorf("wait for chat %s message %s: %w", req.ChatID, sent.MessageID, err)
		}
		out.Response = msg.Response
		summary = fmt.Sprintf("sent message %s to chat %s with response", sent.MessageID, req.ChatID)
	}
	return out, summary, nil
}

func listMessages(ctx context.Context, c *chief.Client, req chatIDRequest) (*chief.MessageList, string, error) {
	list, err := c.Chats.ListMessages(ctx, req.ChatID)
	if err != nil {
		return nil, "", fmt.Errorf("list messages in chat %q: %w", req.ChatID, err)
	}
	return list, fmt.Sprintf("%d message(s) in chat %s", len(list.Messages), req.ChatID), nil
}

func getMessage(ctx context.Context, c *chief.Client, req messageRequest) (*chief.Message, string, error) {
	msg, err := c.Chats.GetMessage(ctx, req.ChatID, req.MessageID)
	if err != nil {
		return nil, "", fmt.Errorf("get message %q in chat %q: %w", req.MessageID, req.ChatID, err)
	}
	return msg, fmt.Sprintf("message %s (response %d chars)", msg.ID, len(msg.Response)), nil
}

func deleteMessage(ctx context.Context, c *chief.Client, req messageRequest) (deleteMessageResponse, string, error) {
	if err := c.Chats.DeleteMessage(ctx, req.ChatID, req.MessageID); err != nil {
		return deleteMessageResponse{}, "", fmt.Errorf("delete message %q in chat %q: %w", req.MessageID, req.ChatID, err)
	}
	return deleteMessageResponse{Deleted: true}, fmt.Sprintf("deleted message %s from chat %s", req.MessageID, req.ChatID), nil
}
