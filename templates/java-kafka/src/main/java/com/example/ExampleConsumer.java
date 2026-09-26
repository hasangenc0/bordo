package {{.GroupId}};

import io.bordo.kafka.BordoKafkaConsumer;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

@Component
public class ExampleConsumer extends BordoKafkaConsumer {

    private static final Logger log = LoggerFactory.getLogger(ExampleConsumer.class);

    @KafkaListener(topics = "${bordo.kafka.topic:{{.KafkaTopic}}}", groupId = "${spring.application.name}")
    public void onMessage(ConsumerRecord<String, String> record) {
        receive(record);
    }

    @Override
    protected void handle(ConsumerRecord<String, String> record) throws Exception {
        log.info("[{{.ProjectName}}] received key={} value={}", record.key(), record.value());
        // TODO: implement your consumer logic here
    }
}
