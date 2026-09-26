package {{.GroupId}};

import io.bordo.job.BordoJob;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Component
public class ExampleJob extends BordoJob {

    private static final Logger log = LoggerFactory.getLogger(ExampleJob.class);

    @Scheduled(cron = "${bordo.job.cron:{{.CronExpression}}}")
    public void run() {
        runWithLock();
    }

    @Override
    protected void execute() throws Exception {
        log.info("[{{.ProjectName}}] executing job");
        // TODO: implement your job logic here
    }
}
