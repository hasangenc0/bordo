package io.bordo.kafka;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "bordo.kafka")
public class KafkaProperties {

    private String bootstrapServers = "localhost:9092";
    private String dlqTopicSuffix = ".dlq";

    public String getBootstrapServers() { return bootstrapServers; }
    public void setBootstrapServers(String bootstrapServers) { this.bootstrapServers = bootstrapServers; }

    public String getDlqTopicSuffix() { return dlqTopicSuffix; }
    public void setDlqTopicSuffix(String dlqTopicSuffix) { this.dlqTopicSuffix = dlqTopicSuffix; }
}
