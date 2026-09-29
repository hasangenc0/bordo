package io.bordo.worker;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "bordo.worker")
public class WorkerProperties {

    private int maxRetries = 3;
    private long backoffMs = 1000;
    private boolean dlqEnabled = true;
    private String queueName = "default";

    public int getMaxRetries() { return maxRetries; }
    public void setMaxRetries(int maxRetries) { this.maxRetries = maxRetries; }

    public long getBackoffMs() { return backoffMs; }
    public void setBackoffMs(long backoffMs) { this.backoffMs = backoffMs; }

    public boolean isDlqEnabled() { return dlqEnabled; }
    public void setDlqEnabled(boolean dlqEnabled) { this.dlqEnabled = dlqEnabled; }

    public String getQueueName() { return queueName; }
    public void setQueueName(String queueName) { this.queueName = queueName; }
}
