package ginmetrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Dbraum/gin-metrics/bloom"
)

var (
	bloomFilter *bloom.BloomFilter
)

// Use set gin metrics middleware
func (m *Monitor) Use(r gin.IRoutes) {
	m.initGinMetrics()

	r.Use(m.monitorInterceptor)
	r.GET(m.metricPath, func(ctx *gin.Context) {
		promhttp.Handler().ServeHTTP(ctx.Writer, ctx.Request)
	})
}

// UseWithoutExposingEndpoint is used to add monitor interceptor to gin router
// It can be called multiple times to intercept from multiple gin.IRoutes
// http path is not set, to do that use Expose function
func (m *Monitor) UseWithoutExposingEndpoint(r gin.IRoutes) {
	m.initGinMetrics()
	r.Use(m.monitorInterceptor)
}

// Expose adds metric path to a given router.
// The router can be different with the one passed to UseWithoutExposingEndpoint.
// This allows to expose metrics on different port.
func (m *Monitor) Expose(r gin.IRoutes) {
	r.GET(m.metricPath, func(ctx *gin.Context) {
		promhttp.Handler().ServeHTTP(ctx.Writer, ctx.Request)
	})
}

// initGinMetrics used to init gin metrics
func (m *Monitor) initGinMetrics() {
	bloomFilter = bloom.NewBloomFilter()

	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        MetricRequestTotal,
		Description: "all the server received request num.",
		Labels:      nil,
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        MetricRequestUVTotal,
		Description: "all the server received ip num.",
		Labels:      nil,
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        MetricURIRequestTotal,
		Description: "all the server received request num with every uri.",
		Labels:      []string{"uri", "method", "code"},
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        MetricRequestBody,
		Description: "the server received request body size, unit byte",
		Labels:      nil,
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        MetricResponseBody,
		Description: "the server send response body size, unit byte",
		Labels:      nil,
	})
	// 请求分布情况
	_ = monitor.AddMetric(&Metric{
		Type:        Histogram,
		Name:        MetricRequestDuration,
		Description: "the time server took to handle the request.",
		Labels:      []string{"uri", "method", "model"},
		Buckets:     m.reqDuration,
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        MetricSlowRequest,
		Description: fmt.Sprintf("the server handled slow requests counter, t=%d.", m.slowTime),
		Labels:      []string{"uri", "method", "code"},
	})
}

// monitorInterceptor as gin monitor middleware.
func (m *Monitor) monitorInterceptor(ctx *gin.Context) {
	if ctx.Request.URL.Path == m.metricPath {
		ctx.Next()
		return
	}
	startTime := time.Now()

	// execute normal process.
	ctx.Next()

	// after request
	m.ginMetricHandle(ctx, startTime)
}

func (m *Monitor) ginMetricHandle(ctx *gin.Context, start time.Time) {
	r := ctx.Request
	w := ctx.Writer

	// set request total
	_ = m.GetMetric(MetricRequestTotal).Inc(nil)

	// set uv
	if clientIP := ctx.ClientIP(); !bloomFilter.Contains(clientIP) {
		bloomFilter.Add(clientIP)
		_ = m.GetMetric(MetricRequestUVTotal).Inc(nil)
	}

	// set uri request total
	_ = m.GetMetric(MetricURIRequestTotal).Inc([]string{ctx.FullPath(), r.Method, strconv.Itoa(w.Status())})

	// set request body size
	// since r.ContentLength can be negative (in some occasions) guard the operation
	if r.ContentLength >= 0 {
		_ = m.GetMetric(MetricRequestBody).Add(nil, float64(r.ContentLength))
	}

	// set slow request
	latency := time.Since(start)
	if int32(latency.Seconds()) > m.slowTime {
		_ = m.GetMetric(MetricSlowRequest).Inc([]string{ctx.FullPath(), r.Method, strconv.Itoa(w.Status())})
	}

	// set request duration
	//_ = m.GetMetric(MetricRequestDuration).Observe([]string{ctx.FullPath()}, latency.Seconds())

	// set response size
	if w.Size() > 0 {
		_ = m.GetMetric(MetricResponseBody).Add(nil, float64(w.Size()))
	}
}
