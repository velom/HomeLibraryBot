package bot

import (
	"context"
	"library/internal/storage/stubs"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// Note: We can't easily mock tgbotapi.BotAPI, so tests focus on internal logic
// without actually sending messages to Telegram

func TestBot_NewBookConversation(t *testing.T) {
	// Setup
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	bot := &Bot{
		api:          nil, // Not needed for internal logic tests
		db:           db,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(), // Use nop logger for tests
	}

	// Create a mock message
	userID := int64(123)
	chatID := int64(456)

	// Step 1: Start /new_book command
	message1 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "/new_book",
	}

	bot.handleNewBookStart(ctx, message1)

	// Verify conversation state
	state, ok := bot.states[userID]
	if !ok {
		t.Fatal("Expected conversation state to be created")
	}
	if state.Command != "new_book" {
		t.Errorf("Expected command 'new_book', got '%s'", state.Command)
	}
	if state.Step != 1 {
		t.Errorf("Expected step 1, got %d", state.Step)
	}

	// Step 2: Provide book name (conversation completes immediately)
	message2 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "Test Book",
	}

	bot.handleNewBookConversation(ctx, message2, state)

	if state.Step != -1 {
		t.Errorf("Expected step -1 (completed), got %d", state.Step)
	}

	// Verify book was created by checking it appears in readable books list
	books, err := db.ListReadableBooks(ctx)
	if err != nil {
		t.Fatalf("Failed to list books: %v", err)
	}
	if len(books) == 0 {
		t.Fatal("Expected book to be created")
	}
	found := false
	for _, book := range books {
		if book.Name == "Test Book" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find 'Test Book' in books list")
	}
}

func TestBot_ReadConversation(t *testing.T) {
	// Setup
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a test book
	_, err := db.CreateBook(ctx, "Test Book")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	bot := &Bot{
		api:          nil, // Not needed for internal logic tests
		db:           db,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(), // Use nop logger for tests
	}

	userID := int64(123)
	chatID := int64(456)

	// Create conversation state simulating custom date selection
	state := &ConversationState{
		Command: "read",
		Step:    1,
		Data:    map[string]interface{}{
			"awaiting_custom_date": true, // Simulate clicking "Custom date" button
		},
	}
	bot.states[userID] = state

	// Step 1: Provide custom date
	message1 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "today",
	}

	bot.handleReadConversation(ctx, message1, state)

	if state.Step != 2 {
		t.Errorf("Expected step 2, got %d", state.Step)
	}
	if _, ok := state.Data["date"].(time.Time); !ok {
		t.Error("Expected date to be set as time.Time")
	}
	if _, ok := state.Data["awaiting_custom_date"]; ok {
		t.Error("Expected awaiting_custom_date flag to be cleared")
	}

	// Note: Steps 2 and 3 (book and participant selection) are now handled
	// via inline keyboard callbacks, not text messages. They would be tested
	// via handleBookCallback and handleParticipantCallback tests.
}

func TestBot_InvalidBookSelection(t *testing.T) {
	// Setup
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a test book
	_, err := db.CreateBook(ctx, "Test Book")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	bot := &Bot{
		api:          nil, // Not needed for internal logic tests
		db:           db,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(), // Use nop logger for tests
	}

	userID := int64(123)
	chatID := int64(456)

	state := &ConversationState{
		Command: "read",
		Step:    2,
		Data: map[string]interface{}{
			"date": time.Now(),
		},
	}
	bot.states[userID] = state

	// Try to select invalid book index
	message := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "999", // Invalid index
	}

	bot.handleReadConversation(ctx, message, state)

	// Should stay on same step
	if state.Step != 2 {
		t.Errorf("Expected to stay on step 2, got %d", state.Step)
	}
}

func TestBot_PanicRecovery(t *testing.T) {
	// Setup
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	bot := &Bot{
		api:          nil, // Not needed for internal logic tests
		db:           db,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(), // Use nop logger for tests
	}

	userID := int64(123)
	chatID := int64(456)

	// Create a state that will cause a panic (missing required data)
	state := &ConversationState{
		Command: "read",
		Step:    3,
		Data:    map[string]interface{}{}, // Missing required fields
	}
	bot.states[userID] = state

	message := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "1",
	}

	// This would panic without recovery - test that it doesn't crash
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handleMessage panicked: %v", r)
		}
	}()

	bot.handleMessage(ctx, message)

	// If we reach here, panic was recovered
	t.Log("Panic was successfully recovered")
}

func TestBot_CommandAfterCallbackCompletion(t *testing.T) {
	// This test verifies the bug fix: after completing a conversation via callback,
	// the next command should be processed correctly
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(), // Use nop logger for tests
	}

	userID := int64(123)
	chatID := int64(456)

	// Simulate a completed conversation state (Step = -1) as would happen after a callback
	bot.states[userID] = &ConversationState{
		Command: "read",
		Step:    -1, // Conversation complete but state not cleaned up
		Data:    map[string]interface{}{},
	}

	// Try to issue a new command - this should work now
	message := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "/start",
	}
	message.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 6},
	}

	// Before the fix, this would call handleConversation and ignore the /start command
	// After the fix, the stale state should be cleaned up and /start should be processed
	bot.handleMessage(ctx, message)

	// Verify the state was cleaned up
	if _, exists := bot.states[userID]; exists {
		t.Error("Expected state to be cleaned up after processing new command")
	}
}

func TestBot_CommandInterruptsConversation(t *testing.T) {
	// Test that any command can interrupt an ongoing conversation
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{123: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(), // Use nop logger for tests
	}

	userID := int64(123)
	chatID := int64(456)

	// Start a /new_book conversation
	message1 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "/new_book",
	}
	message1.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 9},
	}

	bot.handleMessage(ctx, message1)

	// Verify conversation state was created
	if _, exists := bot.states[userID]; !exists {
		t.Fatal("Expected conversation state to be created")
	}

	// Now interrupt with a different command (/start)
	message2 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "/start",
	}
	message2.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 6},
	}

	bot.handleMessage(ctx, message2)

	// Verify the old conversation state was cleaned up
	if _, exists := bot.states[userID]; exists {
		t.Error("Expected conversation state to be deleted when interrupted by new command")
	}

	// Create a book so /read can be started
	_, err := db.CreateBook(ctx, "Test Book")
	if err != nil {
		t.Fatalf("Failed to create book: %v", err)
	}

	// Start a /read conversation and interrupt it too
	message3 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "/read",
	}
	message3.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 5},
	}

	bot.handleMessage(ctx, message3)

	// Verify state exists
	if _, exists := bot.states[userID]; !exists {
		t.Fatal("Expected /read conversation state to be created")
	}

	// Interrupt with /who_is_next
	message4 := &models.Message{
		From: &models.User{ID: userID},
		Chat: models.Chat{ID: chatID},
		Text: "/who_is_next",
	}
	message4.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 12},
	}

	bot.handleMessage(ctx, message4)

	// Verify state was cleaned up
	if _, exists := bot.states[userID]; exists {
		t.Error("Expected /read conversation to be cancelled when interrupted")
	}
}

func TestBot_AuthorizedUserCanSendMessages(t *testing.T) {
	// Test that authorized users can send messages
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	authorizedUserID := int64(123)
	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{authorizedUserID: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(),
	}

	chatID := int64(456)

	// Send /new_book command from authorized user
	message := &models.Message{
		From: &models.User{ID: authorizedUserID, Username: "authorized_user"},
		Chat: models.Chat{ID: chatID},
		Text: "/new_book",
	}
	message.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 9},
	}

	bot.handleMessage(ctx, message)

	// Verify conversation state was created (command was processed)
	if _, exists := bot.states[authorizedUserID]; !exists {
		t.Error("Expected conversation state to be created for authorized user")
	}
}

func TestBot_UnauthorizedUserCannotSendMessages(t *testing.T) {
	// Test that unauthorized users cannot send messages
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	authorizedUserID := int64(123)
	unauthorizedUserID := int64(999)

	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{authorizedUserID: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(),
	}

	chatID := int64(456)

	// Send /start command from unauthorized user
	message := &models.Message{
		From: &models.User{ID: unauthorizedUserID, Username: "unauthorized_user"},
		Chat: models.Chat{ID: chatID},
		Text: "/start",
	}
	message.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 6},
	}

	bot.handleMessage(ctx, message)

	// Verify conversation state was NOT created (command was rejected)
	if _, exists := bot.states[unauthorizedUserID]; exists {
		t.Error("Expected no conversation state for unauthorized user")
	}
}

func TestBot_UnauthorizedUserCannotContinueConversation(t *testing.T) {
	// Test that unauthorized users cannot continue conversations
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	authorizedUserID := int64(123)
	unauthorizedUserID := int64(999)

	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{authorizedUserID: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(),
	}

	chatID := int64(456)

	// Simulate an unauthorized user trying to continue a conversation
	// (even if they somehow had a state, they shouldn't be able to proceed)
	message := &models.Message{
		From: &models.User{ID: unauthorizedUserID, Username: "unauthorized_user"},
		Chat: models.Chat{ID: chatID},
		Text: "Test Book",
	}

	bot.handleMessage(ctx, message)

	// Verify no state was created or modified
	if _, exists := bot.states[unauthorizedUserID]; exists {
		t.Error("Expected no conversation state for unauthorized user")
	}
}

func TestBot_AuthorizedUserCanSendCallbackQueries(t *testing.T) {
	// Test that authorized users can send callback queries
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	authorizedUserID := int64(123)
	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{authorizedUserID: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(),
	}

	// Create a conversation state for the user
	bot.states[authorizedUserID] = &ConversationState{
		Command: "read",
		Step:    1,
		Data:    make(map[string]interface{}),
	}

	// Send callback query from authorized user
	query := &models.CallbackQuery{
		ID:   "callback123",
		From: models.User{ID: authorizedUserID, Username: "authorized_user"},
		Data: "date:today",
		Message: models.MaybeInaccessibleMessage{
			Message: &models.Message{
				Chat: models.Chat{ID: 456},
			},
		},
	}

	// handleCallbackQuery should process this without error
	bot.handleCallbackQuery(ctx, query)

	// Verify state still exists (callback was processed)
	if _, exists := bot.states[authorizedUserID]; !exists {
		t.Error("Expected conversation state to still exist after authorized callback")
	}
}

func TestBot_UnauthorizedUserCannotSendCallbackQueries(t *testing.T) {
	// Test that unauthorized users cannot send callback queries
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	authorizedUserID := int64(123)
	unauthorizedUserID := int64(999)

	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{authorizedUserID: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(),
	}

	// Create a conversation state (simulating a scenario where state exists)
	bot.states[unauthorizedUserID] = &ConversationState{
		Command: "read",
		Step:    1,
		Data:    make(map[string]interface{}),
	}

	// Send callback query from unauthorized user
	query := &models.CallbackQuery{
		ID:   "callback456",
		From: models.User{ID: unauthorizedUserID, Username: "unauthorized_user"},
		Data: "date:today",
		Message: models.MaybeInaccessibleMessage{
			Message: &models.Message{
				Chat: models.Chat{ID: 456},
			},
		},
	}

	// handleCallbackQuery should reject this
	bot.handleCallbackQuery(ctx, query)

	// Verify state still exists (callback was rejected before processing)
	// The state should remain untouched since the callback was rejected
	state, exists := bot.states[unauthorizedUserID]
	if !exists {
		t.Fatal("Expected state to still exist")
	}
	if state.Step != 1 {
		t.Error("Expected state to be unchanged after unauthorized callback")
	}
}

func TestBot_MultipleAuthorizedUsers(t *testing.T) {
	// Test that multiple authorized users can use the bot
	db := stubs.NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	user1 := int64(111)
	user2 := int64(222)
	user3 := int64(333)
	unauthorizedUser := int64(999)

	bot := &Bot{
		api:          nil,
		db:           db,
		allowedUsers: map[int64]bool{user1: true, user2: true, user3: true},
		states:       make(map[int64]*ConversationState),
		logger:       zap.NewNop(),
	}

	chatID := int64(456)

	// User 1 sends command
	message1 := &models.Message{
		From: &models.User{ID: user1, Username: "user1"},
		Chat: models.Chat{ID: chatID},
		Text: "/new_book",
	}
	message1.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 9},
	}
	bot.handleMessage(ctx, message1)

	// User 2 sends command
	message2 := &models.Message{
		From: &models.User{ID: user2, Username: "user2"},
		Chat: models.Chat{ID: chatID},
		Text: "/new_book",
	}
	message2.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 9},
	}
	bot.handleMessage(ctx, message2)

	// User 3 sends command
	message3 := &models.Message{
		From: &models.User{ID: user3, Username: "user3"},
		Chat: models.Chat{ID: chatID},
		Text: "/new_book",
	}
	message3.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 9},
	}
	bot.handleMessage(ctx, message3)

	// Unauthorized user sends command
	messageUnauth := &models.Message{
		From: &models.User{ID: unauthorizedUser, Username: "hacker"},
		Chat: models.Chat{ID: chatID},
		Text: "/start",
	}
	messageUnauth.Entities = []models.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 6},
	}
	bot.handleMessage(ctx, messageUnauth)

	// Verify all authorized users have states
	if _, exists := bot.states[user1]; !exists {
		t.Error("Expected user1 to have conversation state")
	}
	if _, exists := bot.states[user2]; !exists {
		t.Error("Expected user2 to have conversation state")
	}
	if _, exists := bot.states[user3]; !exists {
		t.Error("Expected user3 to have conversation state")
	}

	// Verify unauthorized user does NOT have state
	if _, exists := bot.states[unauthorizedUser]; exists {
		t.Error("Expected unauthorized user to NOT have conversation state")
	}
}
