package io.bordo.job;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.time.Instant;

/**
 * Acquires and releases a row-level distributed lock in the {@code bordo_job_locks} table.
 * Only one node may hold a lock for a given job name at a time.
 */
@Component
public class DistributedLockManager {

    private static final Logger log = LoggerFactory.getLogger(DistributedLockManager.class);

    @Autowired
    private JdbcTemplate jdbc;

    @Autowired
    private JobProperties props;

    /**
     * Try to acquire the lock for {@code jobName}. Returns true if acquired.
     * Expired locks (older than lockTimeoutMs) are reclaimed.
     */
    public boolean tryAcquire(String jobName) {
        long now = Instant.now().toEpochMilli();
        long expiresAt = now + props.getLockTimeoutMs();

        // Reclaim any expired lock first
        jdbc.update(
                "UPDATE bordo_job_locks SET locked_at = ?, expires_at = ?, locked_by = ? " +
                "WHERE job_name = ? AND expires_at < ?",
                now, expiresAt, nodeId(), jobName, now
        );

        // Insert if no row exists yet
        try {
            int rows = jdbc.update(
                    "INSERT INTO bordo_job_locks (job_name, locked_at, expires_at, locked_by) " +
                    "SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM bordo_job_locks WHERE job_name = ?)",
                    jobName, now, expiresAt, nodeId(), jobName
            );
            if (rows > 0) return true;
        } catch (Exception e) {
            log.debug("Lock insert race for job={}: {}", jobName, e.getMessage());
        }

        // Check if we own the lock
        Integer count = jdbc.queryForObject(
                "SELECT COUNT(*) FROM bordo_job_locks WHERE job_name = ? AND locked_by = ? AND expires_at >= ?",
                Integer.class, jobName, nodeId(), now
        );
        return count != null && count > 0;
    }

    /**
     * Release the lock for {@code jobName}.
     */
    public void release(String jobName) {
        jdbc.update("DELETE FROM bordo_job_locks WHERE job_name = ? AND locked_by = ?", jobName, nodeId());
    }

    private String nodeId() {
        return System.getenv().getOrDefault("HOSTNAME", "local");
    }
}
