package io.bordo.data;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.opentelemetry.instrumentation.jdbc.datasource.OpenTelemetryDataSource;
import org.flywaydb.core.Flyway;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;

@Configuration
@EnableConfigurationProperties(DataProperties.class)
@ConditionalOnProperty(name = "bordo.data.url")
public class BordoDataAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public DataSource dataSource(DataProperties props) {
        HikariConfig config = new HikariConfig();
        config.setJdbcUrl(props.getUrl());
        if (props.getUsername() != null) config.setUsername(props.getUsername());
        if (props.getPassword() != null) config.setPassword(props.getPassword());
        config.setMaximumPoolSize(props.getMaximumPoolSize());
        config.setMinimumIdle(props.getMinimumIdle());
        config.setConnectionTimeout(props.getConnectionTimeout());

        DataSource ds = new HikariDataSource(config);
        if (props.isOtelEnabled()) {
            return new OpenTelemetryDataSource(ds);
        }
        return ds;
    }

    @Bean
    @ConditionalOnMissingBean
    public Flyway flyway(DataSource dataSource) {
        Flyway flyway = Flyway.configure()
                .dataSource(dataSource)
                .locations("classpath:db/migration")
                .load();
        flyway.migrate();
        return flyway;
    }

    @Bean
    @ConditionalOnMissingBean
    public JdbcTemplate jdbcTemplate(DataSource dataSource) {
        return new JdbcTemplate(dataSource);
    }
}
