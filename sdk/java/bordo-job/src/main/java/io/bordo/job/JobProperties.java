package io.bordo.job;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "bordo.job")
public class JobProperties {

    private long lockTimeoutMs = 30000;
    private boolean enabled = true;

    public long getLockTimeoutMs() { return lockTimeoutMs; }
    public void setLockTimeoutMs(long lockTimeoutMs) { this.lockTimeoutMs = lockTimeoutMs; }

    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }
}
