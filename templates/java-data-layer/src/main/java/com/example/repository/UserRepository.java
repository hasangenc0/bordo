package {{.GroupId}}.repository;

import {{.GroupId}}.entity.User;
import io.bordo.data.JdbcRepository;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;

import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

@Repository
public class UserRepository extends JdbcRepository<User, UUID> {

    public UserRepository(JdbcTemplate jdbc) {
        super(jdbc);
    }

    @Override
    protected String tableName() {
        return "users";
    }

    @Override
    protected User mapRow(ResultSet rs, int rowNum) throws SQLException {
        Timestamp ts = rs.getTimestamp("created_at");
        return new User(
                UUID.fromString(rs.getString("id")),
                rs.getString("name"),
                rs.getString("email"),
                ts != null ? ts.toInstant() : Instant.now()
        );
    }

    @Override
    protected Map<String, Object> toMap(User user) {
        Map<String, Object> cols = new LinkedHashMap<>();
        cols.put("id", user.getId() != null ? user.getId().toString() : null);
        cols.put("name", user.getName());
        cols.put("email", user.getEmail());
        cols.put("created_at", user.getCreatedAt() != null
                ? Timestamp.from(user.getCreatedAt())
                : Timestamp.from(Instant.now()));
        return cols;
    }

    @Override
    protected UUID getId(User user) {
        return user.getId();
    }

    @Override
    protected UUID parseId(Object raw) {
        return UUID.fromString(raw.toString());
    }
}
