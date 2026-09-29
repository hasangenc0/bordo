package io.bordo.job;

import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.Tracer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Component;

/**
 * Base class for Bordo scheduled jobs. Subclasses call {@link #runWithLock()} from their
 * {@code @Scheduled} method. Provides distributed locking and OTel tracing.
 */
@Component
public abstract class BordoJob {

    private static final Logger log = LoggerFactory.getLogger(BordoJob.class);

    @Autowired
    private DistributedLockManager lockManager;

    @Autowired
    private JobProperties props;

    private Tracer tracer() {
        return GlobalOpenTelemetry.getTracer("io.bordo.job");
    }

    /**
     * The job's business logic. Called while holding the distributed lock.
     */
    protected abstract void execute() throws Exception;

    /**
     * Call this from your {@code @Scheduled} method. Acquires a distributed lock, then
     * executes {@link #execute()}, then releases the lock. Skips silently if another
     * node holds the lock.
     */
    public void runWithLock() {
        if (!props.isEnabled()) return;

        String jobName = getClass().getSimpleName();
        if (!lockManager.tryAcquire(jobName)) {
            log.debug("[{}] lock not acquired, skipping this node", jobName);
            return;
        }

        Span span = tracer().spanBuilder(jobName + ".execute").startSpan();
        try (var scope = span.makeCurrent()) {
            log.info("[{}] starting", jobName);
            execute();
            log.info("[{}] completed", jobName);
        } catch (Exception e) {
            log.error("[{}] failed", jobName, e);
            span.recordException(e);
        } finally {
            lockManager.release(jobName);
            span.end();
        }
    }
}
