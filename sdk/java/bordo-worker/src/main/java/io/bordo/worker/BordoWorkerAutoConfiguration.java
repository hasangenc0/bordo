package io.bordo.worker;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;

@AutoConfiguration
@EnableConfigurationProperties(WorkerProperties.class)
public class BordoWorkerAutoConfiguration {
}
