package {{.GroupId}};

import io.bordo.worker.BordoWorker;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

@Component
public class ExampleWorker extends BordoWorker<String> {

    private static final Logger log = LoggerFactory.getLogger(ExampleWorker.class);

    @Override
    protected void process(String message) {
        log.info("[{{.ProjectName}}] processing message: {}", message);
        // TODO: implement your business logic here
    }
}
