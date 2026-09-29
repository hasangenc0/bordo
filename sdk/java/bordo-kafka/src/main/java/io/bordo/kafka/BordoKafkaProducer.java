package io.bordo.kafka;

import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.support.SendResult;
import org.springframework.stereotype.Component;

import java.util.concurrent.CompletableFuture;

/**
 * Bordo Kafka producer — wraps KafkaTemplate with OTel tracing.
 */
@Component
public class BordoKafkaProducer {

    private static final Logger log = LoggerFactory.getLogger(BordoKafkaProducer.class);

    @Autowired
    private KafkaTemplate<String, String> kafkaTemplate;

    private Tracer tracer() {
        return GlobalOpenTelemetry.getTracer("io.bordo.kafka");
    }

    /**
     * Send a message to the given topic. Returns a CompletableFuture with the send result.
     */
    public CompletableFuture<SendResult<String, String>> send(String topic, String key, String value) {
        Span span = tracer()
                .spanBuilder("BordoKafkaProducer.send")
                .startSpan();
        span.setAttribute("messaging.kafka.topic", topic);
        try (var scope = span.makeCurrent()) {
            CompletableFuture<SendResult<String, String>> future = kafkaTemplate.send(topic, key, value);
            future.whenComplete((result, ex) -> {
                if (ex != null) {
                    log.error("Failed to send to topic={} key={}", topic, key, ex);
                    span.recordException(ex);
                } else {
                    log.debug("Sent to topic={} partition={} offset={}",
                            topic, result.getRecordMetadata().partition(), result.getRecordMetadata().offset());
                }
                span.end();
            });
            return future;
        }
    }
}
