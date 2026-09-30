# Dema Benchmark Results

This result is auto generated from a test environment: Comprehensive performance comparison between **Raw SQLite** (via `zombiezen.com/go/sqlite`) and **Dema ORM**, averaged across **5 consecutive benchmark runs**.

---

## Environment & Methodology

- **OS / Architecture**: Linux `amd64`
- **CPU**: AMD Ryzen 7 PRO 5850U with Radeon Graphics (16 threads)
- **Go Version**: `go1.27.1`
- **SQLite Engine**: ModernC SQLite via `zombiezen.com/go/sqlite`
- **Runs Averaged**: 5 runs per operation (`-count=5 -benchmem`)
- **Metrics Tracked**:
  - **CPU Time**: `ns/op` and delta percentage
  - **Memory Allocated**: `B/op` and delta bytes / percentage
  - **Allocations Count**: `allocs/op` and delta allocations

## 1. In-Memory Database Benchmarks

In-memory benchmarks isolate pure driver and ORM overhead without disk I/O latency.

| Operation | Implementation | Avg Time (ns/op) | CPU Delta | Avg Memory (B/op) | Mem Delta | Avg Allocs/op | Allocs Delta |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| **Select By ID (Cached)** | Raw SQLite | 4,798.6 ns | — | 457 B | — | 9.0 | — |
| | **Dema** | **5,405.2 ns** | **+12.6%** | **530 B** | +73 B (+15.9%) | **11.0** | +2 |
| **Select By ID (Transient)** | Raw SQLite | 37,952.2 ns | — | 857 B | — | 19.0 | — |
| | **Dema** | **40,724.0 ns** | **+7.3%** | **931 B** | +74 B (+8.6%) | **21.0** | +2 |
| **Select Filter (Cached)** | Raw SQLite | 49,547.4 ns | — | 1,419 B | — | 37.0 | — |
| | **Dema** | **51,020.4 ns** | **+3.0%** | **1,503 B** | +84 B (+5.9%) | **39.0** | +2 |
| **Select Filter (Transient)** | Raw SQLite | 98,426.2 ns | — | 1,836 B | — | 47.0 | — |
| | **Dema** | **101,515.2 ns** | **+3.1%** | **1,921 B** | +85 B (+4.6%) | **49.0** | +2 |
| **Insert (Cached)** | Raw SQLite | 11,279.2 ns | — | 393 B | — | 8.0 | — |
| | **Dema** | **9,576.6 ns** | **-15.1%** | **450 B** | +57 B (+14.4%) | **10.0** | +2 |
| **Insert (Transient)** | Raw SQLite | 44,986.0 ns | — | 618 B | — | 11.0 | — |
| | **Dema** | **48,753.2 ns** | **+8.4%** | **675 B** | +57 B (+9.2%) | **13.0** | +2 |
| **Update (Cached)** | Raw SQLite | 5,130.6 ns | — | 353 B | — | 5.0 | — |
| | **Dema** | **5,514.6 ns** | **+7.5%** | **377 B** | +24 B (+6.9%) | **6.0** | +1 |
| **Update (Transient)** | Raw SQLite | 13,991.6 ns | — | 545 B | — | 8.0 | — |
| | **Dema** | **15,595.4 ns** | **+11.5%** | **570 B** | +25 B (+4.5%) | **9.0** | +1 |
| **Upsert (Cached)** | Raw SQLite | 6,118.4 ns | — | 353 B | — | 5.0 | — |
| | **Dema** | **6,539.8 ns** | **+6.9%** | **401 B** | +48 B (+13.7%) | **8.0** | +3 |
| **Upsert (Transient)** | Raw SQLite | 40,879.6 ns | — | 577 B | — | 8.0 | — |
| | **Dema** | **42,330.4 ns** | **+3.5%** | **626 B** | +49 B (+8.5%) | **11.0** | +3 |
| **Count (Cached)** | Raw SQLite | 4,190.6 ns | — | 361 B | — | 6.0 | — |
| | **Dema** | **4,484.4 ns** | **+7.0%** | **386 B** | +25 B (+6.9%) | **7.0** | +1 |
| **Count (Transient)** | Raw SQLite | 9,488.8 ns | — | 722 B | — | 10.0 | — |
| | **Dema** | **10,100.2 ns** | **+6.4%** | **747 B** | +25 B (+3.4%) | **11.0** | +1 |
| **Delete (Cached)** | Raw SQLite | 4,394.2 ns | — | 353 B | — | 5.0 | — |
| | **Dema** | **4,807.4 ns** | **+9.4%** | **382 B** | +29 B (+8.2%) | **7.0** | +2 |
| **Delete (Transient)** | Raw SQLite | 9,627.4 ns | — | 513 B | — | 8.0 | — |
| | **Dema** | **9,142.0 ns** | **-5.0%** | **542 B** | +29 B (+5.6%) | **10.0** | +2 |
| **Insert RETURNING** | Raw SQLite | 16,433.2 ns | — | 457 B | — | 10.0 | — |
| | **Dema** | **17,622.4 ns** | **+7.2%** | **603 B** | +145 B (+31.8%) | **14.0** | +4 |
| **Update RETURNING** | Raw SQLite | 8,425.6 ns | — | 457 B | — | 9.0 | — |
| | **Dema** | **9,225.8 ns** | **+9.5%** | **570 B** | +113 B (+24.7%) | **12.0** | +3 |
| **Delete RETURNING** | Raw SQLite | 5,646.4 ns | — | 401 B | — | 6.0 | — |
| | **Dema** | **6,264.4 ns** | **+10.9%** | **502 B** | +101 B (+25.2%) | **11.0** | +5 |
| **Insert Then Select** | Raw SQLite | 561,799.4 ns | — | 827 B | — | 15.6 | — |
| | **Dema** | **507,553.2 ns** | **-9.7%** | **964 B** | +137 B (+16.6%) | **19.6** | +4 |

## 2. Disk Database Benchmarks

Disk benchmarks reflect real-world file-backed storage with WAL journaling.

| Operation | Implementation | Avg Time (ns/op) | CPU Delta | Avg Memory (B/op) | Mem Delta | Avg Allocs/op | Allocs Delta |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| **Select By ID (Cached)** | Raw SQLite | 7,405.6 ns | — | 457 B | — | 9.0 | — |
| | **Dema** | **8,501.4 ns** | **+14.8%** | **529 B** | +72 B (+15.8%) | **11.0** | +2 |
| **Select By ID (Transient)** | Raw SQLite | 33,987.2 ns | — | 857 B | — | 19.0 | — |
| | **Dema** | **36,781.6 ns** | **+8.2%** | **930 B** | +73 B (+8.5%) | **21.0** | +2 |
| **Select Filter (Cached)** | Raw SQLite | 43,206.4 ns | — | 1,418 B | — | 37.0 | — |
| | **Dema** | **47,191.8 ns** | **+9.2%** | **1,502 B** | +84 B (+5.9%) | **39.0** | +2 |
| **Select Filter (Transient)** | Raw SQLite | 87,835.6 ns | — | 1,834 B | — | 47.0 | — |
| | **Dema** | **90,325.2 ns** | **+2.8%** | **1,920 B** | +85 B (+4.6%) | **49.0** | +2 |
| **Insert (Cached)** | Raw SQLite | 1,683,911.0 ns | — | 402 B | — | 7.0 | — |
| | **Dema** | **1,767,491.8 ns** | **+5.0%** | **461 B** | +59 B (+14.7%) | **9.0** | +2 |
| **Insert (Transient)** | Raw SQLite | 1,754,644.0 ns | — | 628 B | — | 10.0 | — |
| | **Dema** | **1,749,901.2 ns** | **-0.3%** | **682 B** | +54 B (+8.6%) | **12.0** | +2 |
| **Update (Cached)** | Raw SQLite | 14,522.8 ns | — | 353 B | — | 5.0 | — |
| | **Dema** | **16,123.8 ns** | **+11.0%** | **377 B** | +24 B (+6.9%) | **6.0** | +1 |
| **Update (Transient)** | Raw SQLite | 22,860.4 ns | — | 545 B | — | 8.0 | — |
| | **Dema** | **24,088.6 ns** | **+5.4%** | **570 B** | +25 B (+4.6%) | **9.0** | +1 |
| **Upsert (Cached)** | Raw SQLite | 15,692.2 ns | — | 353 B | — | 5.0 | — |
| | **Dema** | **13,744.0 ns** | **-12.4%** | **401 B** | +48 B (+13.7%) | **8.0** | +3 |
| **Upsert (Transient)** | Raw SQLite | 41,566.0 ns | — | 577 B | — | 8.0 | — |
| | **Dema** | **43,066.6 ns** | **+3.6%** | **626 B** | +48 B (+8.4%) | **11.0** | +3 |
| **Count (Cached)** | Raw SQLite | 7,628.6 ns | — | 361 B | — | 6.0 | — |
| | **Dema** | **7,865.0 ns** | **+3.1%** | **385 B** | +24 B (+6.8%) | **7.0** | +1 |
| **Count (Transient)** | Raw SQLite | 14,102.6 ns | — | 721 B | — | 10.0 | — |
| | **Dema** | **14,391.8 ns** | **+2.1%** | **746 B** | +25 B (+3.5%) | **11.0** | +1 |
| **Delete (Cached)** | Raw SQLite | 10,504.4 ns | — | 353 B | — | 5.0 | — |
| | **Dema** | **10,817.2 ns** | **+3.0%** | **381 B** | +28 B (+8.0%) | **7.0** | +2 |
| **Delete (Transient)** | Raw SQLite | 15,650.8 ns | — | 513 B | — | 8.0 | — |
| | **Dema** | **16,653.6 ns** | **+6.4%** | **542 B** | +29 B (+5.6%) | **10.0** | +2 |
| **Insert RETURNING** | Raw SQLite | 1,625,950.2 ns | — | 462 B | — | 9.0 | — |
| | **Dema** | **1,626,504.8 ns** | **+0.0%** | **612 B** | +150 B (+32.5%) | **13.0** | +4 |
| **Update RETURNING** | Raw SQLite | 14,948.6 ns | — | 457 B | — | 9.0 | — |
| | **Dema** | **16,669.8 ns** | **+11.5%** | **570 B** | +113 B (+24.7%) | **12.0** | +3 |
| **Delete RETURNING** | Raw SQLite | 12,662.0 ns | — | 401 B | — | 6.0 | — |
| | **Dema** | **13,875.6 ns** | **+9.6%** | **501 B** | +100 B (+25.0%) | **11.0** | +5 |