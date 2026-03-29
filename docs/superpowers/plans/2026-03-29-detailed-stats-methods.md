# Detailed Stats Methods Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two new storage methods returning detailed cross-tab reading statistics (book-centric and participant-centric), with optional date range and entity filters, and expose them as LLM tools.

**Architecture:** Both methods return flat rows from a CROSS JOIN between books and participants, LEFT JOINed to events. Date filters go into the JOIN condition to preserve zero counts. Book/participant filters go into WHERE. LLM tool handlers group flat rows into readable text output.

**Tech Stack:** Go, ClickHouse, existing storage interface pattern

---

### Task 1: Add new model types

**Files:**
- Modify: `internal/models/models.go`

- [ ] **Step 1: Add DetailedBookStat and ParticipantBookStat types**

Add after the existing `RareBookStat` struct:

```go
// DetailedBookStat represents per-participant reading statistics for a book
type DetailedBookStat struct {
	BookName        string     `json:"bookName"`
	ParticipantName string     `json:"participantName"`
	ReadCount       int        `json:"readCount"`
	LastReadDate    *time.Time `json:"lastReadDate"` // nil if never read
}

// ParticipantBookStat represents per-book reading statistics for a participant
type ParticipantBookStat struct {
	ParticipantName string `json:"participantName"`
	BookName        string `json:"bookName"`
	ReadCount       int    `json:"readCount"`
}
```

- [ ] **Step 2: Verify build**

Run: `make build`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```
feat: add DetailedBookStat and ParticipantBookStat model types
```

---

### Task 2: Add new methods to Storage interface

**Files:**
- Modify: `internal/storage/storage.go`

- [ ] **Step 1: Add GetDetailedBookStats and GetParticipantStats to the interface**

Add to the `// Statistics operations` section, after `GetRarelyReadBooks`:

```go
	// GetDetailedBookStats returns per-participant reading statistics for books.
	// For each book × participant combination: read count and last read date.
	// Zero-value times mean no date bound. Empty strings mean no filter.
	// Results ordered by book_name ASC, read_count DESC, participant_name ASC.
	GetDetailedBookStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.DetailedBookStat, error)

	// GetParticipantStats returns per-book reading statistics for participants.
	// For each participant × book combination: read count.
	// Zero-value times mean no date bound. Empty strings mean no filter.
	// Results ordered by participant_name ASC, read_count DESC, book_name ASC.
	GetParticipantStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.ParticipantBookStat, error)
```

- [ ] **Step 2: Verify the file compiles (will fail until implementations exist)**

Run: `go vet ./internal/storage/`
Expected: SUCCESS (interface only)

- [ ] **Step 3: Commit**

```
feat: add GetDetailedBookStats and GetParticipantStats to Storage interface
```

---

### Task 3: Implement mock storage methods + tests

**Files:**
- Modify: `internal/storage/stubs/mock.go`
- Modify: `internal/storage/stubs/mock_test.go`

- [ ] **Step 1: Write failing tests for GetDetailedBookStats**

Add to `mock_test.go`:

```go
func TestMockDB_GetDetailedBookStats(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Create events: Alice reads "The Hobbit" twice, Bob reads it once
	now := time.Now()
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -5), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -2), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -1), "The Hobbit", "Bob")

	// Get all stats (no filters)
	stats, err := db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Should have entries for every book × participant combination
	participants, _ := db.ListParticipants(ctx)
	books, _ := db.ListReadableBooks(ctx)
	expectedRows := len(books) * len(participants)
	if len(stats) != expectedRows {
		t.Errorf("Expected %d rows (books×participants), got %d", expectedRows, len(stats))
	}

	// Find Alice + The Hobbit: should be 2 reads
	found := false
	for _, s := range stats {
		if s.BookName == "The Hobbit" && s.ParticipantName == "Alice" {
			found = true
			if s.ReadCount != 2 {
				t.Errorf("Expected Alice to have read The Hobbit 2 times, got %d", s.ReadCount)
			}
			if s.LastReadDate == nil {
				t.Error("Expected non-nil LastReadDate for Alice + The Hobbit")
			}
			break
		}
	}
	if !found {
		t.Error("Expected to find Alice + The Hobbit in stats")
	}

	// Filter by book
	stats, err = db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "The Hobbit", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != len(participants) {
		t.Errorf("Expected %d rows for one book, got %d", len(participants), len(stats))
	}

	// Filter by participant
	stats, err = db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != len(books) {
		t.Errorf("Expected %d rows for one participant, got %d", len(books), len(stats))
	}

	// Filter by date range (only last 3 days)
	since := now.AddDate(0, 0, -3)
	stats, err = db.GetDetailedBookStats(ctx, since, now, "The Hobbit", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(stats))
	}
	if stats[0].ReadCount != 1 {
		t.Errorf("Expected 1 read in last 3 days, got %d", stats[0].ReadCount)
	}

	// Zero-read entries: find a participant who never read The Hobbit
	stats, err = db.GetDetailedBookStats(ctx, time.Time{}, time.Time{}, "The Hobbit", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	hasZero := false
	for _, s := range stats {
		if s.ReadCount == 0 {
			hasZero = true
			if s.LastReadDate != nil {
				t.Error("Expected nil LastReadDate for zero-read entry")
			}
			break
		}
	}
	if !hasZero {
		t.Error("Expected at least one participant with 0 reads for The Hobbit")
	}
}

func TestMockDB_GetParticipantStats(t *testing.T) {
	db := NewMockDB()
	ctx := context.Background()
	if err := db.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	now := time.Now()
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -5), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -2), "The Hobbit", "Alice")
	_ = db.CreateEvent(ctx, now.AddDate(0, 0, -1), "Goodnight Moon", "Alice")

	// Get all stats
	stats, err := db.GetParticipantStats(ctx, time.Time{}, time.Time{}, "", "")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	participants, _ := db.ListParticipants(ctx)
	books, _ := db.ListReadableBooks(ctx)
	expectedRows := len(books) * len(participants)
	if len(stats) != expectedRows {
		t.Errorf("Expected %d rows, got %d", expectedRows, len(stats))
	}

	// Find Alice + The Hobbit
	for _, s := range stats {
		if s.ParticipantName == "Alice" && s.BookName == "The Hobbit" {
			if s.ReadCount != 2 {
				t.Errorf("Expected 2 reads, got %d", s.ReadCount)
			}
			break
		}
	}

	// Filter by participant
	stats, err = db.GetParticipantStats(ctx, time.Time{}, time.Time{}, "", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	if len(stats) != len(books) {
		t.Errorf("Expected %d rows for Alice, got %d", len(books), len(stats))
	}

	// Ordered by participant_name ASC, read_count DESC, book_name ASC
	if len(stats) > 1 {
		if stats[0].ReadCount < stats[1].ReadCount {
			t.Error("Expected stats ordered by read_count DESC within participant")
		}
	}

	// Filter by date range
	since := now.AddDate(0, 0, -3)
	stats, err = db.GetParticipantStats(ctx, since, now, "", "Alice")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	for _, s := range stats {
		if s.BookName == "The Hobbit" && s.ReadCount != 1 {
			t.Errorf("Expected 1 read in last 3 days for The Hobbit, got %d", s.ReadCount)
		}
	}

	// Zero reads present
	hasZero := false
	for _, s := range stats {
		if s.ReadCount == 0 {
			hasZero = true
			break
		}
	}
	if !hasZero {
		t.Error("Expected zero-read entries")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestMockDB_GetDetailedBookStats|TestMockDB_GetParticipantStats" ./internal/storage/stubs/`
Expected: FAIL (methods not implemented)

- [ ] **Step 3: Implement GetDetailedBookStats in mock**

Add to `mock.go` before `Close()`:

```go
func (m *MockDB) GetDetailedBookStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.DetailedBookStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type key struct{ book, participant string }
	counts := make(map[key]int)
	lastDates := make(map[key]time.Time)

	for _, event := range m.events {
		if !startDate.IsZero() && event.Date.Before(startDate) {
			continue
		}
		if !endDate.IsZero() && event.Date.After(endDate) {
			continue
		}
		k := key{event.BookName, event.ParticipantName}
		counts[k]++
		if d, ok := lastDates[k]; !ok || event.Date.After(d) {
			lastDates[k] = event.Date
		}
	}

	var stats []models.DetailedBookStat
	for _, book := range m.books {
		if !book.IsReadable {
			continue
		}
		if bookName != "" && book.Name != bookName {
			continue
		}
		for _, p := range m.sortedParticipants() {
			if participantName != "" && p.Name != participantName {
				continue
			}
			k := key{book.Name, p.Name}
			stat := models.DetailedBookStat{
				BookName:        book.Name,
				ParticipantName: p.Name,
				ReadCount:       counts[k],
			}
			if d, ok := lastDates[k]; ok {
				stat.LastReadDate = &d
			}
			stats = append(stats, stat)
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].BookName != stats[j].BookName {
			return stats[i].BookName < stats[j].BookName
		}
		if stats[i].ReadCount != stats[j].ReadCount {
			return stats[i].ReadCount > stats[j].ReadCount
		}
		return stats[i].ParticipantName < stats[j].ParticipantName
	})

	return stats, nil
}
```

Note: `sortedParticipants()` is a helper that returns participants sorted by name. Check if it exists; if not, extract from ListParticipants logic. The mock already sorts in ListParticipants — extract a `sortedParticipants()` helper or inline the sort.

- [ ] **Step 4: Implement GetParticipantStats in mock**

Add to `mock.go` before `Close()`:

```go
func (m *MockDB) GetParticipantStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.ParticipantBookStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type key struct{ participant, book string }
	counts := make(map[key]int)

	for _, event := range m.events {
		if !startDate.IsZero() && event.Date.Before(startDate) {
			continue
		}
		if !endDate.IsZero() && event.Date.After(endDate) {
			continue
		}
		k := key{event.ParticipantName, event.BookName}
		counts[k]++
	}

	var stats []models.ParticipantBookStat
	for _, p := range m.sortedParticipants() {
		if participantName != "" && p.Name != participantName {
			continue
		}
		for _, book := range m.books {
			if !book.IsReadable {
				continue
			}
			if bookName != "" && book.Name != bookName {
				continue
			}
			k := key{p.Name, book.Name}
			stats = append(stats, models.ParticipantBookStat{
				ParticipantName: p.Name,
				BookName:        book.Name,
				ReadCount:       counts[k],
			})
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ParticipantName != stats[j].ParticipantName {
			return stats[i].ParticipantName < stats[j].ParticipantName
		}
		if stats[i].ReadCount != stats[j].ReadCount {
			return stats[i].ReadCount > stats[j].ReadCount
		}
		return stats[i].BookName < stats[j].BookName
	})

	return stats, nil
}
```

- [ ] **Step 5: Add sortedParticipants helper if needed**

If `m.sortedParticipants()` doesn't exist, add this helper to `mock.go`:

```go
func (m *MockDB) sortedParticipants() []models.Participant {
	var participants []models.Participant
	for _, p := range m.participants {
		participants = append(participants, p)
	}
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].Name < participants[j].Name
	})
	return participants
}
```

- [ ] **Step 6: Run tests**

Run: `go test -v -run "TestMockDB_GetDetailedBookStats|TestMockDB_GetParticipantStats" ./internal/storage/stubs/`
Expected: PASS

- [ ] **Step 7: Run all tests**

Run: `make test`
Expected: PASS

- [ ] **Step 8: Commit**

```
feat: implement GetDetailedBookStats and GetParticipantStats in mock storage
```

---

### Task 4: Implement ClickHouse storage methods

**Files:**
- Modify: `internal/storage/ch/clickhouse.go`

- [ ] **Step 1: Implement GetDetailedBookStats**

Add before `Close()` method:

```go
func (db *ClickHouseDB) GetDetailedBookStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.DetailedBookStat, error) {
	var conditions []string
	var joinConditions []string
	var args []interface{}

	conditions = append(conditions, "b.is_readable = true")

	if bookName != "" {
		conditions = append(conditions, "b.name = ?")
		args = append(args, bookName)
	}
	if participantName != "" {
		conditions = append(conditions, "p.name = ?")
		args = append(args, participantName)
	}
	if !startDate.IsZero() {
		joinConditions = append(joinConditions, "e.date >= ?")
		args = append(args, startDate)
	}
	if !endDate.IsZero() {
		joinConditions = append(joinConditions, "e.date <= ?")
		args = append(args, endDate)
	}

	joinOn := "b.name = e.book_name AND p.name = e.participant_name"
	if len(joinConditions) > 0 {
		joinOn += " AND " + strings.Join(joinConditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT
			b.name AS book_name,
			p.name AS participant_name,
			toInt32(count(e.date)) AS read_count,
			max(e.date) AS last_read_date
		FROM books b
		CROSS JOIN participants p
		LEFT JOIN events e ON %s
		WHERE %s
		GROUP BY b.name, p.name
		ORDER BY b.name ASC, read_count DESC, p.name ASC
	`, joinOn, strings.Join(conditions, " AND "))

	// Reorder args: WHERE args first, then JOIN args won't work because
	// ClickHouse processes args positionally. We need: WHERE args, then JOIN args.
	// Actually, the query has JOIN before WHERE, so args order = JOIN args then WHERE args.
	// Let's rebuild args in correct order.

	// Rebuild args in query order: JOIN conditions come before WHERE in SQL
	// but we added WHERE args first. Fix: build args in SQL order.
	args = nil
	if !startDate.IsZero() {
		args = append(args, startDate)
	}
	if !endDate.IsZero() {
		args = append(args, endDate)
	}
	if bookName != "" {
		args = append(args, bookName)
	}
	if participantName != "" {
		args = append(args, participantName)
	}

	rows, err := db.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get detailed book stats: %w", err)
	}
	defer rows.Close()

	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	var stats []models.DetailedBookStat
	for rows.Next() {
		var bName, pName string
		var readCount int32
		var lastReadDate time.Time
		if err := rows.Scan(&bName, &pName, &readCount, &lastReadDate); err != nil {
			return nil, fmt.Errorf("failed to scan detailed book stat: %w", err)
		}
		var lastReadPtr *time.Time
		if lastReadDate.After(epoch) {
			lastReadPtr = &lastReadDate
		}
		stats = append(stats, models.DetailedBookStat{
			BookName:        bName,
			ParticipantName: pName,
			ReadCount:       int(readCount),
			LastReadDate:    lastReadPtr,
		})
	}
	return stats, nil
}
```

- [ ] **Step 2: Implement GetParticipantStats**

```go
func (db *ClickHouseDB) GetParticipantStats(ctx context.Context, startDate, endDate time.Time, bookName, participantName string) ([]models.ParticipantBookStat, error) {
	var conditions []string
	var joinConditions []string

	conditions = append(conditions, "b.is_readable = true")

	if bookName != "" {
		conditions = append(conditions, "b.name = ?")
	}
	if participantName != "" {
		conditions = append(conditions, "p.name = ?")
	}
	if !startDate.IsZero() {
		joinConditions = append(joinConditions, "e.date >= ?")
	}
	if !endDate.IsZero() {
		joinConditions = append(joinConditions, "e.date <= ?")
	}

	joinOn := "p.name = e.participant_name AND b.name = e.book_name"
	if len(joinConditions) > 0 {
		joinOn += " AND " + strings.Join(joinConditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT
			p.name AS participant_name,
			b.name AS book_name,
			toInt32(count(e.date)) AS read_count
		FROM participants p
		CROSS JOIN books b
		LEFT JOIN events e ON %s
		WHERE %s
		GROUP BY p.name, b.name
		ORDER BY p.name ASC, read_count DESC, b.name ASC
	`, joinOn, strings.Join(conditions, " AND "))

	// Build args in SQL order: JOIN args (dates) then WHERE args (names)
	var args []interface{}
	if !startDate.IsZero() {
		args = append(args, startDate)
	}
	if !endDate.IsZero() {
		args = append(args, endDate)
	}
	if bookName != "" {
		args = append(args, bookName)
	}
	if participantName != "" {
		args = append(args, participantName)
	}

	rows, err := db.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get participant stats: %w", err)
	}
	defer rows.Close()

	var stats []models.ParticipantBookStat
	for rows.Next() {
		var pName, bName string
		var readCount int32
		if err := rows.Scan(&pName, &bName, &readCount); err != nil {
			return nil, fmt.Errorf("failed to scan participant stat: %w", err)
		}
		stats = append(stats, models.ParticipantBookStat{
			ParticipantName: pName,
			BookName:        bName,
			ReadCount:       int(readCount),
		})
	}
	return stats, nil
}
```

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```
feat: implement GetDetailedBookStats and GetParticipantStats in ClickHouse storage
```

---

### Task 5: Add LLM tools

**Files:**
- Modify: `internal/bot/ask.go`

- [ ] **Step 1: Add tool definitions to askTools**

Add two new tools to the `askTools` slice (before the closing `}`):

```go
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_detailed_book_stats",
			Description: "Получить детальную статистику по книгам: кто сколько раз прочитал каждую книгу и когда был последний раз. Возвращает все комбинации книга×участник, включая нулевые. Для общей картины вызывай без фильтров, для конкретной книги — с параметром book",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"since":{"type":"string","description":"Дата начала периода YYYY-MM-DD (по умолчанию: всё время)"},
				"until":{"type":"string","description":"Дата конца периода YYYY-MM-DD (по умолчанию: всё время)"},
				"book":{"type":"string","description":"Название книги для фильтрации (по умолчанию: все книги)"},
				"participant":{"type":"string","description":"Имя участника для фильтрации (по умолчанию: все участники)"}
			}}`),
		},
	},
	{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_participant_stats",
			Description: "Получить статистику в разрезе читающих: кто сколько каких книг прочитал. Возвращает все комбинации участник×книга, включая нулевые. Для конкретного участника — с параметром participant",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"since":{"type":"string","description":"Дата начала периода YYYY-MM-DD (по умолчанию: всё время)"},
				"until":{"type":"string","description":"Дата конца периода YYYY-MM-DD (по умолчанию: всё время)"},
				"book":{"type":"string","description":"Название книги для фильтрации (по умолчанию: все книги)"},
				"participant":{"type":"string","description":"Имя участника для фильтрации (по умолчанию: все участники)"}
			}}`),
		},
	},
```

- [ ] **Step 2: Add case handlers in executeTool switch**

Add to the `switch name` block in `executeTool`:

```go
	case "get_detailed_book_stats":
		since := stringArg(args, "since", "")
		until := stringArg(args, "until", "")
		book := stringArg(args, "book", "")
		participant := stringArg(args, "participant", "")
		return b.toolGetDetailedBookStats(ctx, since, until, book, participant)
	case "get_participant_stats":
		since := stringArg(args, "since", "")
		until := stringArg(args, "until", "")
		book := stringArg(args, "book", "")
		participant := stringArg(args, "participant", "")
		return b.toolGetParticipantStats(ctx, since, until, book, participant)
```

- [ ] **Step 3: Implement toolGetDetailedBookStats handler**

Add after `toolGetLabels`:

```go
func (b *Bot) toolGetDetailedBookStats(ctx context.Context, since, until, book, participant string) string {
	sinceDate, untilDate, errStr := parseDateRange(since, until)
	if errStr != "" {
		return errStr
	}

	stats, err := b.db.GetDetailedBookStats(ctx, sinceDate, untilDate, book, participant)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	if len(stats) == 0 {
		return "(нет данных)\n"
	}

	var sb strings.Builder
	currentBook := ""
	for _, s := range stats {
		if s.BookName != currentBook {
			if currentBook != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("📚 %s:\n", s.BookName))
			currentBook = s.BookName
		}
		if s.LastReadDate != nil {
			sb.WriteString(fmt.Sprintf("  %s — %d раз, последний: %s\n",
				s.ParticipantName, s.ReadCount, s.LastReadDate.Format("2006-01-02")))
		} else {
			sb.WriteString(fmt.Sprintf("  %s — 0 раз\n", s.ParticipantName))
		}
	}
	return sb.String()
}
```

- [ ] **Step 4: Implement toolGetParticipantStats handler**

```go
func (b *Bot) toolGetParticipantStats(ctx context.Context, since, until, book, participant string) string {
	sinceDate, untilDate, errStr := parseDateRange(since, until)
	if errStr != "" {
		return errStr
	}

	stats, err := b.db.GetParticipantStats(ctx, sinceDate, untilDate, book, participant)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	if len(stats) == 0 {
		return "(нет данных)\n"
	}

	var sb strings.Builder
	currentParticipant := ""
	for _, s := range stats {
		if s.ParticipantName != currentParticipant {
			if currentParticipant != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("👤 %s:\n", s.ParticipantName))
			currentParticipant = s.ParticipantName
		}
		if s.ReadCount > 0 {
			sb.WriteString(fmt.Sprintf("  %s — %d раз\n", s.BookName, s.ReadCount))
		} else {
			sb.WriteString(fmt.Sprintf("  %s — 0 раз\n", s.BookName))
		}
	}
	return sb.String()
}
```

- [ ] **Step 5: Extract parseDateRange helper**

To avoid duplicating date parsing logic, add a helper:

```go
func parseDateRange(since, until string) (sinceDate, untilDate time.Time, errStr string) {
	if since != "" {
		var err error
		sinceDate, err = time.Parse("2006-01-02", since)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Sprintf("error: invalid 'since' date format %q, expected YYYY-MM-DD", since)
		}
	}
	if until != "" {
		var err error
		untilDate, err = time.Parse("2006-01-02", until)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Sprintf("error: invalid 'until' date format %q, expected YYYY-MM-DD", until)
		}
		untilDate = untilDate.Add(24*time.Hour - time.Second)
	}
	return sinceDate, untilDate, ""
}
```

Also refactor `toolGetLastEvents` to use `parseDateRange` to avoid duplication.

- [ ] **Step 6: Verify build and tests**

Run: `make build && make test`
Expected: SUCCESS

- [ ] **Step 7: Commit**

```
feat: add get_detailed_book_stats and get_participant_stats LLM tools
```
