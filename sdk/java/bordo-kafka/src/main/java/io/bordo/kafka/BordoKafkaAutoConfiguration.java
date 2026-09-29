package io.bordo.kafka;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.kafka.core.KafkaTemplate;

@AutoConfiguration
@EnableConfigurationProperties(KafkaProperties.class)
public class BordoKafkaAutoConfiguration {

    @Bean
    public BordoKafkaProducer bordoKafkaProducer(KafkaTemplate<String, String> kafkaTemplate) {
        return new BordoKafkaProducer();
    }
}
