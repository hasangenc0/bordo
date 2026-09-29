package io.bordo.worker;

import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;

/**
 * Base class for Bordo workers. Subclasses implement {@link #process(Object)} and
 * receive retry logic, DLQ routing, and OTel tracing for free.
 *
 * @param <T> the message type this worker processes
 */
public abstract class BordoWorker<T> {

    private static final Logger log = LoggerFactory.getLogger(BordoWorker.class);

    @Autowired
    private WorkerProperties props;

    private Tracer tracer() {
        return GlobalOpenTelemetry.getTracer("io.bordo.worker");
    }

    /**
     * Process a single message. Called by {@link #handle(Object)} after span/retry setup.
     */
    protected abstract void process(T message) throws Exception;

    /**
     * Handle a message: create an OTel span, attempt up to maxRetries times, route to DLQ on exhaustion.
     */
    public void handle(T message) {
        Span span = tracer().spanBuilder(getClass().getSimpleName() + ".handle").startSpan();
        try (var scope = span.makeCurrent()) {
            attempt(message, 0);
        } finally {
            span.end();
        }
    }

    private void attempt(T message, int attempt) {
        try {
            process(message);
        } catch (Exception e) {
            if (attempt < props.getMaxRetries()) {
                log.warn("[{}] attempt {}/{} failed, retrying in {}ms: {}",
                        getClass().getSimpleName(), attempt + 1, props.getMaxRetries(),
                        props.getBackoffMs(), e.getMessage());
                try {
                    Thread.sleep(props.getBackoffMs() * (attempt + 1));
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                }
                attempt(message, attempt + 1);
            } else {
                log.error("[{}] all {} retries exhausted", getClass().getSimpleName(), props.getMaxRetries(), e);
                if (props.isDlqEnabled()) {
                    routeToDlq(message, e);
                }
            }
        }
    }

    /**
     * Send a failed message to the DLQ. Override to use a real queue client.
     */
    protected void routeToDlq(T message, Exception cause) {
        log.error("[{}] routing to DLQ ({}.dlq): {}",
                getClass().getSimpleName(), props.getQueueName(), message, cause);
    }
}
