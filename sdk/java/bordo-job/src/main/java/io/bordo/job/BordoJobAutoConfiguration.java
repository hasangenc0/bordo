package io.bordo.job;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.scheduling.annotation.EnableScheduling;

@AutoConfiguration
@EnableConfigurationProperties(JobProperties.class)
@EnableScheduling
public class BordoJobAutoConfiguration {

    @Bean
    public DistributedLockManager distributedLockManager(JdbcTemplate jdbc) {
        return new DistributedLockManager();
    }

    @Bean
    public JobSchemaInitializer jobSchemaInitializer(JdbcTemplate jdbc) {
        return new JobSchemaInitializer(jdbc);
    }
}
