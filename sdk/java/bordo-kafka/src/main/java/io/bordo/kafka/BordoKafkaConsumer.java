package io.bordo.kafka;

import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.core.KafkaTemplate;

/**
 * Base class for Bordo Kafka consumers. Subclasses implement {@link #handle(ConsumerRecord)}.
 * Provides OTel tracing and automatic DLQ routing on failure.
 */
public abstract class BordoKafkaConsumer {

    private static final Logger log = LoggerFactory.getLogger(BordoKafkaConsumer.class);

    @Autowired(required = false)
    private KafkaTemplate<String, String> kafkaTemplate;

    @Autowired
    private KafkaProperties props;

    private Tracer tracer() {
        return GlobalOpenTelemetry.getTracer("io.bordo.kafka");
    }

    /**
     * Process a single Kafka record. Implement business logic here.
     */
    protected abstract void handle(ConsumerRecord<String, String> record) throws Exception;

    /**
     * Called by Spring Kafka listener infrastructure. Wraps handle() with tracing and DLQ routing.
     */
    public void receive(ConsumerRecord<String, String> record) {
        Span span = tracer()
                .spanBuilder(getClass().getSimpleName() + ".receive")
                .startSpan();
        try (var scope = span.makeCurrent()) {
            span.setAttribute("messaging.kafka.topic", record.topic());
            span.setAttribute("messaging.kafka.partition", record.partition());
            handle(record);
        } catch (Exception e) {
            log.error("[{}] failed to process record from topic={} partition={} offset={}",
                    getClass().getSimpleName(), record.topic(), record.partition(), record.offset(), e);
            span.recordException(e);
            routeToDlq(record, e);
        } finally {
            span.end();
        }
    }

    private void routeToDlq(ConsumerRecord<String, String> record, Exception cause) {
        String dlqTopic = record.topic() + props.getDlqTopicSuffix();
        if (kafkaTemplate != null) {
            kafkaTemplate.send(dlqTopic, record.key(), record.value());
            log.warn("[{}] sent failed record to DLQ topic={}", getClass().getSimpleName(), dlqTopic);
        } else {
            log.error("[{}] no KafkaTemplate available, dropping failed record for DLQ topic={}",
                    getClass().getSimpleName(), dlqTopic, cause);
        }
    }
}
