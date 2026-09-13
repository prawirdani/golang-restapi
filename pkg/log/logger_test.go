package log

// Benchmarks for the logger adapters. Construct the adapter with a bool
// (production flag) — no application config is required.
//
// BenchmarkZerolog_Info-8            20172             59824 ns/op             304 B/op          4 allocs/op
// func BenchmarkZerolog_Info(b *testing.B) {
// 	b.ReportAllocs()
//
// 	logger := NewZerologAdapter(true)
//
// 	for b.Loop() {
// 		logger.Info("benchmark message", "foo", "bar")
// 	}
// }
//
// 19593             57208 ns/op             384 B/op          8 allocs/op
// func BenchmarkSlog_Info(b *testing.B) {
// 	b.ReportAllocs()
//
// 	logger := NewSlogAdapter(true)
//
// 	for i := 0; i < b.N; i++ {
// 		logger.Info("benchmark message", "foo", "bar")
// 	}
// }
