package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/myrjola/petrapp/internal/chatbot"
)

type chatListTemplateData struct {
	BaseTemplateData
	Conversations []chatbot.Conversation
}

type chatConversationTemplateData struct {
	BaseTemplateData
	Conversation chatbot.Conversation
	Messages     []chatbot.ChatMessage
}

// chatGET handles GET requests for the conversation list page.
func (app *application) chatGET(w http.ResponseWriter, r *http.Request) {
	// Get all conversations for the current user
	conversations, err := app.chatbotService.GetUserConversations(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := chatListTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Conversations:    conversations,
	}

	app.render(w, r, http.StatusOK, "chat-list", data)
}

// chatConversationGET handles GET requests for viewing a specific conversation.
func (app *application) chatConversationGET(w http.ResponseWriter, r *http.Request) {
	// Parse conversation ID from URL path
	conversationIDStr := r.PathValue("conversationID")
	conversationID, err := strconv.Atoi(conversationIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get the conversation
	conversation, err := app.chatbotService.GetConversation(r.Context(), conversationID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get all messages for the conversation
	messages, err := app.chatbotService.GetConversationMessages(r.Context(), conversationID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := chatConversationTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Conversation:     conversation,
		Messages:         messages,
	}

	app.render(w, r, http.StatusOK, "chat-conversation", data)
}

// chatMessagePOST handles POST requests for sending a message in a conversation.
func (app *application) chatMessagePOST(w http.ResponseWriter, r *http.Request) {
	// Parse conversation ID from URL path
	conversationIDStr := r.PathValue("conversationID")
	conversationID, err := strconv.Atoi(conversationIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form data
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Get message content from form
	content := r.PostForm.Get("content")
	if content == "" {
		app.serverError(w, r, errors.New("message content not provided"))
		return
	}

	// Send the message and get AI response
	_, err = app.chatbotService.SendMessage(r.Context(), conversationID, content)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Follow POST-Redirect-GET pattern - redirect to conversation view
	redirect(w, r, fmt.Sprintf("/chat/%d", conversationID))
}
