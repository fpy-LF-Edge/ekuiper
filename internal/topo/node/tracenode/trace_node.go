package tracenode

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/lf-edge/ekuiper/contract/v2/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	topoContext "github.com/lf-edge/ekuiper/v2/internal/topo/context"
	"github.com/lf-edge/ekuiper/v2/internal/xsql"
	"github.com/lf-edge/ekuiper/v2/pkg/tracer"
)

const (
	DataKey = "data"
	RuleKey = "rule"
)

func TraceRowTuple(ctx api.StreamContext, input *xsql.RawTuple, opName string) (bool, api.StreamContext, trace.Span) {
	if !ctx.IsTraceEnabled() {
		return false, nil, nil
	}
	if !checkCtxByStrategy(ctx, input.GetTracerCtx()) {
		return false, nil, nil
	}
	spanCtx, span := tracer.GetTracer().Start(input.GetTracerCtx(), opName)
	x := topoContext.WithContext(spanCtx)
	return true, x, span
}

func RecordRowOrCollection(input interface{}, span trace.Span) {
	switch d := input.(type) {
	case xsql.Row:
		span.SetAttributes(attribute.String(DataKey, ToStringRow(d)))
	case xsql.Collection:
		if d.Len() > 0 {
			span.SetAttributes(attribute.String(DataKey, ToStringCollection(d)))
		}
	case *xsql.RawTuple:
		span.SetAttributes(attribute.String(DataKey, string(d.Rawdata)))
	}
}

func RecordSpanData(input any, span trace.Span) {
	switch d := input.(type) {
	case []byte:
		span.SetAttributes(attribute.String(DataKey, string(d)))
	}
}

func TraceInput(ctx api.StreamContext, d interface{}, opName string, opts ...trace.SpanStartOption) (bool, api.StreamContext, trace.Span) {
	if !ctx.IsTraceEnabled() {
		return false, nil, nil
	}
	input, ok := d.(xsql.HasTracerCtx)
	if !ok {
		return false, nil, nil
	}
	if !checkCtxByStrategy(ctx, input.GetTracerCtx()) {
		return false, nil, nil
	}
	spanCtx, span := tracer.GetTracer().Start(input.GetTracerCtx(), opName, opts...)
	span.SetAttributes(attribute.String(RuleKey, ctx.GetRuleId()))
	x := topoContext.WithContext(spanCtx)
	input.SetTracerCtx(x)
	return true, x, span
}

func TraceRow(ctx api.StreamContext, input xsql.Row, opName string, opts ...trace.SpanStartOption) (bool, api.StreamContext, trace.Span) {
	if !ctx.IsTraceEnabled() {
		return false, nil, nil
	}
	if !checkCtxByStrategy(ctx, input.GetTracerCtx()) {
		return false, nil, nil
	}
	spanCtx, span := tracer.GetTracer().Start(input.GetTracerCtx(), opName, opts...)
	span.SetAttributes(attribute.String(RuleKey, ctx.GetRuleId()))
	x := topoContext.WithContext(spanCtx)
	input.SetTracerCtx(x)
	return true, x, span
}

func StartTraceBySpanCtx(ctx, sctx api.StreamContext, opName string) (bool, api.StreamContext, trace.Span) {
	if !ctx.IsTraceEnabled() {
		return false, nil, nil
	}
	if !checkCtxByStrategy(ctx, sctx) {
		return false, nil, nil
	}
	spanCtx, span := tracer.GetTracer().Start(sctx, opName)
	span.SetAttributes(attribute.String(RuleKey, ctx.GetRuleId()))
	ingestCtx := topoContext.WithContext(spanCtx)
	return true, ingestCtx, span
}

func StartTraceBackground(ctx api.StreamContext, opName string) (bool, api.StreamContext, trace.Span) {
	if !ctx.IsTraceEnabled() {
		return false, nil, nil
	}
	if !checkCtxByStrategy(ctx, ctx) {
		return false, nil, nil
	}
	spanCtx, span := tracer.GetTracer().Start(context.Background(), opName)
	ruleID := ctx.GetRuleId()
	span.SetAttributes(attribute.String(RuleKey, ruleID))
	ingestCtx := topoContext.WithContext(spanCtx)
	return true, ingestCtx, span
}

func StartTraceByID(ctx api.StreamContext, traceID [16]byte, spanID [8]byte) (bool, api.StreamContext, trace.Span) {
	if !ctx.IsTraceEnabled() {
		return false, nil, nil
	}
	carrier := map[string]string{
		"traceparent": buildTraceParent(traceID, spanID),
	}
	propagator := propagation.TraceContext{}
	traceCtx := propagator.Extract(context.Background(), propagation.MapCarrier(carrier))
	spanCtx, span := tracer.GetTracer().Start(traceCtx, ctx.GetOpId())
	span.SetAttributes(attribute.String(RuleKey, ctx.GetRuleId()))
	ingestCtx := topoContext.WithContext(spanCtx)
	return true, ingestCtx, span
}

func ToStringRow(r xsql.Row) string {
	d := r.Clone().ToMap()
	b, _ := json.Marshal(d)
	return string(b)
}

func ToStringCollection(r xsql.Collection) string {
	d := r.Clone().ToMaps()
	b, _ := json.Marshal(d)
	return string(b)
}

func buildTraceParent(traceID [16]byte, spanID [8]byte) string {
	return fmt.Sprintf("00-%s-%s-01", hex.EncodeToString(traceID[:]), hex.EncodeToString(spanID[:]))
}

func checkCtxByStrategy(ctx, tracerCtx api.StreamContext) bool {
	strategy := extractStrategy(ctx)
	switch strategy {
	case topoContext.AlwaysTraceStrategy:
		return true
	case topoContext.HeadTraceStrategy:
		return hasTraceContext(tracerCtx)
	}
	return false
}

func extractStrategy(ctx api.StreamContext) topoContext.TraceStrategy {
	dctx, ok := ctx.(*topoContext.DefaultContext)
	if !ok {
		return topoContext.AlwaysTraceStrategy
	}
	return dctx.GetStrategy()
}

func hasTraceContext(ctx context.Context) bool {
	spanContext := trace.SpanContextFromContext(ctx)
	return spanContext.IsValid()
}
