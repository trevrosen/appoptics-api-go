package appoptics

import (
	"fmt"
	"path"
	"time"

	ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type instrumentedServer struct {
	m       *MeasurementSet
	service string
	method  string
	tags    map[string]interface{}
}

func newInstrumentedServer(m *MeasurementSet, service, method string, tags map[string]interface{}) *instrumentedServer {
	return &instrumentedServer{
		m:       m,
		service: service,
		method:  method,
		tags:    tags,
	}
}

func (s *instrumentedServer) key(key string) string {
	return MetricWithTags(fmt.Sprintf("%s.%s.%s", s.service, s.method, key), s.tags)
}

func (s *instrumentedServer) sent() {
	s.m.Incr(s.key("sent"))
}

func (s *instrumentedServer) received() {
	s.m.Incr(s.key("received"))
}

func (s *instrumentedServer) handled(err error) {
	s.m.Incr(s.key(grpc.Code(err).String()))
}

func (s *instrumentedServer) timed(t int64) {
	s.m.UpdateGaugeValue(s.key("time_ms"), t)
}

func UnaryServerInterceptor(m *MeasurementSet) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		instrument := newInstrumentedServer(
			m,
			path.Dir(info.FullMethod)[1:],
			path.Base(info.FullMethod),
			ctxtags.Extract(ctx).Values(),
		)

		instrument.received()

		start := time.Now()
		resp, err := handler(ctx, req)
		durationMs := time.Now().Sub(start) / time.Millisecond

		instrument.timed(int64(durationMs))
		instrument.handled(err)

		if err == nil {
			instrument.sent()
		}

		return resp, err
	}
}

func StreamServerInterceptor(m *MeasurementSet) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		instrument := &instrumentedServerStream{
			ss,
			newInstrumentedServer(
				m,
				path.Dir(info.FullMethod)[1:],
				path.Base(info.FullMethod),
				nil,
			),
		}

		start := time.Now()
		err := handler(srv, instrument)
		durationMs := time.Now().Sub(start) / time.Millisecond

		instrument.handled(err)
		instrument.timed(int64(durationMs))

		return err
	}
}

type instrumentedServerStream struct {
	grpc.ServerStream
	*instrumentedServer
}

func (s *instrumentedServerStream) SendMsg(m interface{}) error {
	err := s.ServerStream.SendMsg(m)
	if err == nil {
		s.instrumentedServer.sent()
	}
	return err
}

func (s *instrumentedServerStream) RecvMsg(m interface{}) error {
	err := s.ServerStream.RecvMsg(m)
	if err == nil {
		s.instrumentedServer.received()
	}
	return err
}
