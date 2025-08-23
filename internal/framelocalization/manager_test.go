package framelocalization

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
)

// Test suite extending FrameBaseTestSuite
type FrameLocalizationTestSuite struct {
	frametests.FrameBaseTestSuite
}

func TestFrameLocalizationTestSuite(t *testing.T) {
	testSuite := &FrameLocalizationTestSuite{}
	// Initialize the InitResourceFunc to satisfy the requirement
	testSuite.InitResourceFunc = func(ctx context.Context) []definition.TestResource {
		// Return empty slice since we're testing the localization manager directly
		return []definition.TestResource{}
	}
	suite.Run(t, testSuite)
}

// Test table-driven tests for localization functionality
func (suite *FrameLocalizationTestSuite) TestLocalizationManagerFunctionality() {
	tests := []struct {
		name string
		test func(ctx context.Context, suite *FrameLocalizationTestSuite)
	}{
		{"NoopConfigDefaultLanguage", suite.testNoopConfigDefaultLanguage},
		{"NoopConfigCustomLanguage", suite.testNoopConfigCustomLanguage},
		{"NewNoopManager", suite.testNewNoopManager},
		{"NoopManagerTranslation", suite.testNoopManagerTranslation},
		{"NoopManagerClose", suite.testNoopManagerClose},
		{"ManagerInterfaceCompliance", suite.testManagerInterfaceCompliance},
		{"ConfigInterfaceCompliance", suite.testConfigInterfaceCompliance},
		{"ConcurrentTranslations", suite.testConcurrentTranslations},
		{"TranslationWithArgs", suite.testTranslationWithArgs},
		{"ContextPropagation", suite.testContextPropagation},
		{"EdgeCases", suite.testEdgeCases},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ctx := context.Background()
			tt.test(ctx, suite)
		})
	}
}

func (suite *FrameLocalizationTestSuite) testNoopConfigDefaultLanguage(ctx context.Context, _ *FrameLocalizationTestSuite) {
	// Test with empty language (should default to "en")
	config := NoopConfig{Lang: ""}
	suite.Equal("en", config.DefaultLanguage())
	
	// Test with nil-like behavior
	var emptyConfig NoopConfig
	suite.Equal("en", emptyConfig.DefaultLanguage())
}

func (suite *FrameLocalizationTestSuite) testNoopConfigCustomLanguage(ctx context.Context, _ *FrameLocalizationTestSuite) {
	// Test with custom languages
	testCases := []struct {
		lang     string
		expected string
	}{
		{"fr", "fr"},
		{"es", "es"},
		{"de", "de"},
		{"ja", "ja"},
		{"zh-CN", "zh-CN"},
		{"en-US", "en-US"},
		{"pt-BR", "pt-BR"},
	}
	
	for _, tc := range testCases {
		config := NoopConfig{Lang: tc.lang}
		suite.Equal(tc.expected, config.DefaultLanguage())
	}
}

func (suite *FrameLocalizationTestSuite) testNewNoopManager(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	suite.NotNil(manager)
	suite.NotNil(manager.cfg)
	suite.Equal("en", manager.cfg.DefaultLanguage())
	
	// Test with different config
	frenchConfig := NoopConfig{Lang: "fr"}
	frenchManager := NewNoopManager(frenchConfig)
	suite.Equal("fr", frenchManager.cfg.DefaultLanguage())
}

func (suite *FrameLocalizationTestSuite) testNoopManagerTranslation(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	// Test basic translation (should return key as-is)
	testCases := []string{
		"hello",
		"welcome.message",
		"error.not_found",
		"user.profile.title",
		"",
		"key-with-dashes",
		"key_with_underscores",
		"key.with.dots",
		"UPPERCASE_KEY",
		"123numeric456",
		"special!@#$%characters",
	}
	
	for _, key := range testCases {
		result := manager.T(ctx, key)
		suite.Equal(key, result, "Translation should return key as-is for key: %s", key)
	}
}

func (suite *FrameLocalizationTestSuite) testNoopManagerClose(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	// Test close operation (should not return error)
	err := manager.Close(ctx)
	suite.NoError(err)
	
	// Test close with different contexts
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	
	err = manager.Close(ctxWithTimeout)
	suite.NoError(err)
	
	// Test close with cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	
	err = manager.Close(cancelledCtx)
	suite.NoError(err)
}

func (suite *FrameLocalizationTestSuite) testManagerInterfaceCompliance(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	// Verify that NoopManager implements Manager interface
	var _ Manager = manager
	
	// Test all Manager interface methods
	translation := manager.T(ctx, "test.key")
	suite.Equal("test.key", translation)
	
	err := manager.Close(ctx)
	suite.NoError(err)
}

func (suite *FrameLocalizationTestSuite) testConfigInterfaceCompliance(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "test"}
	
	// Verify that NoopConfig implements Config interface
	var _ Config = config
	
	// Test Config interface method
	lang := config.DefaultLanguage()
	suite.Equal("test", lang)
}

func (suite *FrameLocalizationTestSuite) testConcurrentTranslations(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	// Test concurrent access to translation method
	done := make(chan bool, 3)
	keys := []string{"key1", "key2", "key3"}
	
	for i, key := range keys {
		go func(index int, k string) {
			defer func() { done <- true }()
			
			for j := 0; j < 50; j++ {
				result := manager.T(ctx, k)
				suite.Equal(k, result)
			}
		}(i, key)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			suite.Fail("Test timed out")
		}
	}
}

func (suite *FrameLocalizationTestSuite) testTranslationWithArgs(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	// Test translation with various argument types
	// Note: NoopManager ignores args and returns key as-is
	testCases := []struct {
		key  string
		args []any
	}{
		{"hello", []any{"world"}},
		{"user.greeting", []any{"John", "Doe"}},
		{"number.format", []any{42, 3.14}},
		{"boolean.test", []any{true, false}},
		{"mixed.args", []any{"string", 123, true, 3.14}},
		{"no.args", []any{}},
		{"nil.args", nil},
	}
	
	for _, tc := range testCases {
		result := manager.T(ctx, tc.key, tc.args...)
		suite.Equal(tc.key, result, "Translation should return key as-is regardless of args")
	}
}

func (suite *FrameLocalizationTestSuite) testContextPropagation(ctx context.Context, _ *FrameLocalizationTestSuite) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	// Test with different context types
	contexts := []context.Context{
		context.Background(),
		context.TODO(),
		ctx,
	}
	
	// Test with context values
	ctxWithValue := context.WithValue(ctx, "test-key", "test-value")
	contexts = append(contexts, ctxWithValue)
	
	// Test with timeout context
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	contexts = append(contexts, ctxWithTimeout)
	
	for i, testCtx := range contexts {
		result := manager.T(testCtx, "test.key")
		suite.Equal("test.key", result, "Context %d should not affect translation", i)
		
		err := manager.Close(testCtx)
		suite.NoError(err, "Context %d should not affect close operation", i)
	}
}

func (suite *FrameLocalizationTestSuite) testEdgeCases(ctx context.Context, _ *FrameLocalizationTestSuite) {
	// Test with nil config (should not panic)
	suite.NotPanics(func() {
		manager := &NoopManager{cfg: nil}
		_ = manager.T(ctx, "test")
		_ = manager.Close(ctx)
	})
	
	// Test with very long keys
	longKey := string(make([]byte, 1000))
	for i := range longKey {
		longKey = longKey[:i] + "a" + longKey[i+1:]
	}
	
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	
	result := manager.T(ctx, longKey)
	suite.Equal(longKey, result)
	
	// Test with unicode keys
	unicodeKeys := []string{
		"测试键",
		"тестовый ключ",
		"مفتاح الاختبار",
		"テストキー",
		"🔑🌍🚀",
		"key.with.émojis.🎉",
	}
	
	for _, key := range unicodeKeys {
		result := manager.T(ctx, key)
		suite.Equal(key, result, "Unicode key should be handled correctly: %s", key)
	}
}

// Additional test for multiple manager instances
func (suite *FrameLocalizationTestSuite) TestMultipleManagerInstances() {
	ctx := context.Background()
	
	// Create multiple managers with different configurations
	configs := []NoopConfig{
		{Lang: "en"},
		{Lang: "fr"},
		{Lang: "es"},
		{Lang: "de"},
		{Lang: ""},
	}
	
	managers := make([]*NoopManager, len(configs))
	for i, config := range configs {
		managers[i] = NewNoopManager(config)
	}
	
	// Test that each manager maintains its own configuration
	expectedLangs := []string{"en", "fr", "es", "de", "en"}
	for i, manager := range managers {
		suite.Equal(expectedLangs[i], manager.cfg.DefaultLanguage())
		
		// Test translation behavior is consistent
		result := manager.T(ctx, "test.key")
		suite.Equal("test.key", result)
		
		// Test close behavior
		err := manager.Close(ctx)
		suite.NoError(err)
	}
}

// Benchmark tests for performance validation
func BenchmarkNoopManager_T(b *testing.B) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.T(ctx, "benchmark.key")
	}
}

func BenchmarkNoopManager_TWithArgs(b *testing.B) {
	config := NoopConfig{Lang: "en"}
	manager := NewNoopManager(config)
	ctx := context.Background()
	args := []any{"arg1", 42, true}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.T(ctx, "benchmark.key", args...)
	}
}

func BenchmarkNoopConfig_DefaultLanguage(b *testing.B) {
	config := NoopConfig{Lang: "en"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.DefaultLanguage()
	}
}

func BenchmarkNewNoopManager(b *testing.B) {
	config := NoopConfig{Lang: "en"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewNoopManager(config)
	}
}
