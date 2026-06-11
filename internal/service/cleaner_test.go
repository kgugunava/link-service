package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kgugunava/link-service/internal/domain"
)

type mockCleanerRepo struct {
	mock.Mock
}

func (m *mockCleanerRepo) Save(ctx context.Context, url *domain.URL) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}

func (m *mockCleanerRepo) GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error) {
	args := m.Called(ctx, shortCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.URL), args.Error(1)
}

func (m *mockCleanerRepo) DeleteExpired(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func newTestCleaner(t *testing.T, repo URLRepositoryInterface, interval time.Duration) *Cleaner {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewCleaner(repo, interval, logger)
}

func TestNewCleaner(t *testing.T) {
	repo := new(mockCleanerRepo)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("creates cleaner with all fields set", func(t *testing.T) {
		cleaner := NewCleaner(repo, time.Minute, logger)
		require.NotNil(t, cleaner)
		assert.Equal(t, repo, cleaner.repo)
		assert.Equal(t, time.Minute, cleaner.interval)
		assert.Equal(t, logger, cleaner.logger)
		assert.NotNil(t, cleaner.stopCh)
	})

	t.Run("uses default logger when nil is passed", func(t *testing.T) {
		cleaner := NewCleaner(repo, time.Minute, nil)
		require.NotNil(t, cleaner)
		assert.NotNil(t, cleaner.logger)
	})
}

func TestCleaner_Run(t *testing.T) {
	t.Run("calls DeleteExpired periodically", func(t *testing.T) {
		repo := new(mockCleanerRepo)
		repo.On("DeleteExpired", mock.Anything).
			Return(int64(0), nil).
			Maybe()

		cleaner := newTestCleaner(t, repo, 100*time.Millisecond)

		done := make(chan struct{})
		go func() {
			cleaner.Run(context.Background())
			close(done)
		}()

		time.Sleep(350 * time.Millisecond)
		cleaner.Stop()

		<-done
		repo.AssertNumberOfCalls(t, "DeleteExpired", 3)
	})

	t.Run("stops on Stop() signal", func(t *testing.T) {
		repo := new(mockCleanerRepo)
		repo.On("DeleteExpired", mock.Anything).Return(int64(0), nil).Maybe()

		cleaner := newTestCleaner(t, repo, 50*time.Millisecond)

		done := make(chan struct{})
		go func() {
			cleaner.Run(context.Background())
			close(done)
		}()

		time.Sleep(120 * time.Millisecond)
		cleaner.Stop()

		select {
		case <-done:
			// OK
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Run did not stop after Stop() call")
		}
	})

	t.Run("stops on context cancellation", func(t *testing.T) {
		repo := new(mockCleanerRepo)
		repo.On("DeleteExpired", mock.Anything).Return(int64(0), nil).Maybe()

		cleaner := newTestCleaner(t, repo, 50*time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			cleaner.Run(ctx)
			close(done)
		}()

		time.Sleep(120 * time.Millisecond)
		cancel()

		select {
		case <-done:
			// OK
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Run did not stop after context cancellation")
		}
	})

	t.Run("continues running after DeleteExpired error", func(t *testing.T) {
		repo := new(mockCleanerRepo)
		repo.On("DeleteExpired", mock.Anything).
			Return(int64(0), errors.New("db connection failed")).
			Once()
		repo.On("DeleteExpired", mock.Anything).
			Return(int64(5), nil).
			Once()
		repo.On("DeleteExpired", mock.Anything).
			Return(int64(0), nil).
			Maybe()

		cleaner := newTestCleaner(t, repo, 100*time.Millisecond)

		done := make(chan struct{})
		go func() {
			cleaner.Run(context.Background())
			close(done)
		}()

		time.Sleep(350 * time.Millisecond)
		cleaner.Stop()
		<-done

		repo.AssertNumberOfCalls(t, "DeleteExpired", 3)
	})

	t.Run("logs deleted count when greater than zero", func(t *testing.T) {
		repo := new(mockCleanerRepo)
		repo.On("DeleteExpired", mock.Anything).
			Return(int64(42), nil).
			Once()
		repo.On("DeleteExpired", mock.Anything).
			Return(int64(0), nil).
			Maybe()

		cleaner := newTestCleaner(t, repo, 50*time.Millisecond)

		done := make(chan struct{})
		go func() {
			cleaner.Run(context.Background())
			close(done)
		}()

		time.Sleep(100 * time.Millisecond)
		cleaner.Stop()
		<-done

		repo.AssertExpectations(t)
	})
}

func TestCleaner_ConcurrentStop(t *testing.T) {
	repo := new(mockCleanerRepo)
	repo.On("DeleteExpired", mock.Anything).Return(int64(0), nil).Maybe()

	cleaner := newTestCleaner(t, repo, 50*time.Millisecond)

	go cleaner.Run(context.Background())
	time.Sleep(80 * time.Millisecond)

	var wg sync.WaitGroup
	panics := int32(0)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt32(&panics, 1)
				}
			}()
			cleaner.Stop()
		}()
	}

	wg.Wait()
	t.Logf("panics: %d out of 5 concurrent Stop() calls", panics)
}

func TestCleaner_FullLifecycle(t *testing.T) {
	repo := new(mockCleanerRepo)

	repo.On("DeleteExpired", mock.Anything).
		Return(int64(5), nil).Once()
	repo.On("DeleteExpired", mock.Anything).
		Return(int64(0), nil).Once()
	repo.On("DeleteExpired", mock.Anything).
		Return(int64(2), nil).Once()
	repo.On("DeleteExpired", mock.Anything).
		Return(int64(0), nil).Maybe()

	cleaner := newTestCleaner(t, repo, 100*time.Millisecond)

	done := make(chan struct{})
	go func() {
		cleaner.Run(context.Background())
		close(done)
	}()

	time.Sleep(350 * time.Millisecond)
	cleaner.Stop()
	<-done

	repo.AssertNumberOfCalls(t, "DeleteExpired", 3)
}

func TestCleaner_StopBeforeRun(t *testing.T) {
	repo := new(mockCleanerRepo)
	cleaner := newTestCleaner(t, repo, 50*time.Millisecond)

	cleaner.Stop()

	done := make(chan struct{})
	go func() {
		cleaner.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not stop when stopCh was closed before start")
	}

	repo.AssertNotCalled(t, "DeleteExpired")
}

func TestCleaner_RunWithCancelledContext(t *testing.T) {
	repo := new(mockCleanerRepo)
	cleaner := newTestCleaner(t, repo, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		cleaner.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not stop with cancelled context")
	}

	repo.AssertNotCalled(t, "DeleteExpired")
}