# Metrics

`pkg/metrics` renders small inline summaries that fit in a list row, a table
cell or an inspector value. Every function returns foreground-only colored
text (via `ansi.CellColor`), so a table's selected-row background passes
through intact.

```
Badge(12, 3, 1)      12✓ 3⚠ 1✗
Ratio(3, 4)          3/4
Bar(0.6, 1, 10)      ▰▰▰▰▰▰▱▱▱▱
Spark(cpu, 12)       ▁▃▂▅▇▅▄▆█▅▃▂
```

## Functions

| Function | Renders | Color |
|---|---|---|
| `Badge(ok, warn, down int)` | `12✓ 3⚠ 1✗`; zero buckets omitted, all-zero is `""` | green / yellow / red |
| `Ratio(done, total int)` | `3/4` | green when full, yellow partial, red at zero, gray when total is 0 |
| `Bar(value, max float64, width int)` | a fixed-width bar filled to `value/max` | green full, yellow partial, red empty |
| `BarStyled(value, max, width, color int)` | the same, in one ANSI color | your choice |
| `Spark(values []float64, width int)` | an 8-level sparkline, resampled to `width` | blue |
| `SparkStyled(values, width, color int)` | the same, in one ANSI color | your choice |

Colors are 256-color palette indices. Use the `Styled` variants when a
threshold doesn't mean good or bad (CPU, memory).

## Usage

The package only renders: you own the data and any history. For a sparkline,
keep a fixed-size window per row and pass it each refresh:

```go
s.cpu[name] = append(s.cpu[name], sample)
if len(s.cpu[name]) > 30 {
    s.cpu[name] = s.cpu[name][1:]
}

rows = append(rows, table.KeyedRow{Key: name, Cells: []string{
    name,
    metrics.Ratio(d.Ready, d.Replicas),
    metrics.Badge(d.OK, d.Warn, d.Down),
    metrics.Bar(d.CPU, 1, 10),
    metrics.Spark(s.cpu[name], 12),
}})
```

`Badge` has a variable width; give its column a fixed `Width` if you need
alignment.

See the workloads pane in `examples/liveops`, polled every 2s.
