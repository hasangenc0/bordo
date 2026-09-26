package {{.GroupId}};

import io.bordo.kafka.BordoKafkaProducer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

@Component
public class ExampleProducer {

    private static final Logger log = LoggerFactory.getLogger(ExampleProducer.class);

    @Autowired
    private BordoKafkaProducer producer;

    @Value("${bordo.kafka.topic:{{.KafkaTopic}}}")
    private String topic;

    public void send(String key, String payload) {
        producer.send(topic, key, payload)
                .whenComplete((result, ex) -> {
                    if (ex != null) {
                        log.error("[{{.ProjectName}}] failed to send key={}", key, ex);
                    } else {
                        log.info("[{{.ProjectName}}] sent key={}", key);
                    }
                });
    }
}
