package repo_test

import (
	"testing"
	"time"

	"cdpnetool/internal/logger"
	"cdpnetool/internal/storage/db"
	"cdpnetool/internal/storage/model"
	"cdpnetool/internal/storage/repo"
	"cdpnetool/pkg/domain"
)

// setupEventTestDB 创建用于 EventRepo 测试的内存数据库。
func setupEventTestDB(t *testing.T) *repo.EventRepo {
	gdb, err := db.New(db.Options{
		FullPath: ":memory:",
		Prefix:   "test_",
	})
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}

	err = db.Migrate(gdb, &model.NetworkEventRecord{})
	if err != nil {
		t.Fatalf("迁移数据库失败: %v", err)
	}

	// 使用生产 logger 但不配置输出，用于测试
	l := logger.New(logger.Options{Level: "disabled"})
	// 使用较小的批量大小和较短的刷新间隔以便测试
	return repo.NewEventRepo(gdb, l, repo.EventRepoOptions{
		BatchSize:     5,
		FlushInterval: 100 * time.Millisecond,
		MaxBufferSize: 100,
	})
}

// TestEventRepo_AsyncWrite 测试异步批量写入是否正常工作。
func TestEventRepo_AsyncWrite(t *testing.T) {
	r := setupEventTestDB(t)
	defer r.Stop()

	// 创建多个测试事件
	for i := 0; i < 10; i++ {
		evt := &domain.NetworkEvent{
			Session:   "test-session",
			Target:    "test-target",
			IsMatched: true,
			Request: domain.RequestInfo{
				URL:    "http://example.com",
				Method: "GET",
			},
			Response: domain.ResponseInfo{
				StatusCode: 200,
			},
			FinalResult: "passed",
			Timestamp:   time.Now().UnixMilli(),
		}
		r.Record(evt)
	}

	// 等待异步写入完成
	time.Sleep(200 * time.Millisecond)

	// 验证数据是否写入数据库
	events, total, err := r.Query(repo.QueryOptions{
		SessionID: "test-session",
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("查询事件失败: %v", err)
	}

	if total != 10 {
		t.Errorf("预期写入 10 条记录，实际为 %d", total)
	}

	if len(events) != 10 {
		t.Errorf("预期查询到 10 条记录，实际为 %d", len(events))
	}
}

// TestEventRepo_QueryWithFilters 测试查询功能的过滤条件。
func TestEventRepo_QueryWithFilters(t *testing.T) {
	r := setupEventTestDB(t)
	defer r.Stop()

	// 插入不同类型的事件
	events := []*domain.NetworkEvent{
		{
			Session:     "s1",
			IsMatched:   true,
			Request:     domain.RequestInfo{URL: "http://a.com", Method: "GET"},
			Response:    domain.ResponseInfo{StatusCode: 200},
			FinalResult: "passed",
			Timestamp:   1000,
		},
		{
			Session:     "s1",
			IsMatched:   true,
			Request:     domain.RequestInfo{URL: "http://b.com", Method: "POST"},
			Response:    domain.ResponseInfo{StatusCode: 403},
			FinalResult: "blocked",
			Timestamp:   2000,
		},
		{
			Session:     "s2",
			IsMatched:   true,
			Request:     domain.RequestInfo{URL: "http://c.com", Method: "GET"},
			Response:    domain.ResponseInfo{StatusCode: 200},
			FinalResult: "modified",
			Timestamp:   3000,
		},
	}

	for _, evt := range events {
		r.Record(evt)
	}

	time.Sleep(200 * time.Millisecond)

	// 按 SessionID 过滤
	_, total, _ := r.Query(repo.QueryOptions{
		SessionID: "s1",
		Limit:     100,
	})
	if total != 2 {
		t.Errorf("SessionID 过滤预期 2 条，实际 %d", total)
	}

	// 按 FinalResult 过滤
	results, total, _ := r.Query(repo.QueryOptions{
		FinalResult: "blocked",
		Limit:       100,
	})
	if total != 1 {
		t.Errorf("FinalResult 过滤预期 1 条，实际 %d", total)
	}
	if len(results) > 0 && results[0].FinalResult != "blocked" {
		t.Errorf("过滤结果不符，预期 blocked，实际 %s", results[0].FinalResult)
	}

	// 按 Method 过滤
	_, total, _ = r.Query(repo.QueryOptions{
		Method: "POST",
		Limit:  100,
	})
	if total != 1 {
		t.Errorf("Method 过滤预期 1 条，实际 %d", total)
	}
}
