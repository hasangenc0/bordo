package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	brokers := splitAndTrim(firstNonEmpty(os.Getenv("KAFKA_BOOTSTRAP_SERVERS"), "localhost:9092"))
	topic := firstNonEmpty(os.Getenv("KAFKA_TOPIC"), "{{.Extra.KafkaTopic}}", "events")
	groupID := firstNonEmpty(os.Getenv("KAFKA_GROUP_ID"), "{{.ProjectName}}", "my-kafka-service")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go serveHealth(logger)
	go consume(ctx, logger, brokers, topic, groupID)
	go produce(ctx, logger, brokers, topic)

	logger.Info("{{.ProjectName}} kafka service started",
		"brokers", brokers, "topic", topic, "group", groupID)

	<-ctx.Done()
	logger.Info("shutting down")
}

// consume reads messages from the topic and handles each one.
func consume(ctx context.Context, logger *slog.Logger, brokers []string, topic, groupID string) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	defer r.Close()

	for {
		m, err := r.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("read error", "err", err)
			time.Sleep(time.Second)
			continue
		}
		logger.Info("[{{.ProjectName}}] received", "key", string(m.Key), "value", string(m.Value))
		// TODO: implement your consumer logic here.
	}
}

// produce sends a heartbeat message every 10s as a working example.
func produce(ctx context.Context, logger *slog.Logger, brokers []string, topic string) {
	w := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	defer w.Close()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			msg := kafka.Message{
				Key:   []byte("{{.ProjectName}}"),
				Value: []byte("heartbeat at " + t.UTC().Format(time.RFC3339)),
			}
			if err := w.WriteMessages(ctx, msg); err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Error("write error", "err", err)
				continue
			}
			logger.Info("[{{.ProjectName}}] sent heartbeat")
		}
	}
}

func serveHealth(logger *slog.Logger) {
	port := firstNonEmpty(os.Getenv("PORT"), "8080")
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /actuator/health", ok)

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("health server error", "err", err)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func splitAndTrim(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
