package io.bordo.job;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;

import jakarta.annotation.PostConstruct;

/**
 * Creates the {@code bordo_job_locks} table on startup if it doesn't exist.
 */
public class JobSchemaInitializer {

    private static final Logger log = LoggerFactory.getLogger(JobSchemaInitializer.class);

    private final JdbcTemplate jdbc;

    public JobSchemaInitializer(JdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    @PostConstruct
    public void init() {
        jdbc.execute("""
                CREATE TABLE IF NOT EXISTS bordo_job_locks (
                    job_name   VARCHAR(255) PRIMARY KEY,
                    locked_at  BIGINT NOT NULL,
                    expires_at BIGINT NOT NULL,
                    locked_by  VARCHAR(255) NOT NULL
                )
                """);
        log.debug("bordo_job_locks table ready");
    }
}
