package io.bordo.data;

import org.springframework.jdbc.core.JdbcTemplate;

import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.stream.Collectors;

/**
 * Abstract JDBC-backed repository providing standard CRUD via JdbcTemplate.
 *
 * <p>Subclasses implement five methods: {@link #tableName()}, {@link #mapRow},
 * {@link #toMap}, {@link #getId}, and {@link #parseId}. The {@code toMap} result
 * must include an {@code "id"} key; if the value is {@code null}, a random UUID
 * string is substituted before INSERT.
 */
public abstract class JdbcRepository<T, ID> implements Repository<T, ID> {

    protected final JdbcTemplate jdbc;

    protected JdbcRepository(JdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    protected abstract String tableName();

    protected abstract T mapRow(ResultSet rs, int rowNum) throws SQLException;

    /** Return a column map including {@code "id"}; null id means auto-generate. */
    protected abstract Map<String, Object> toMap(T entity);

    /** Extract the typed ID from an entity; null if the entity is not yet persisted. */
    protected abstract ID getId(T entity);

    /** Convert a raw Object id value (typically a String) to the typed ID. */
    protected abstract ID parseId(Object raw);

    @Override
    public T findById(ID id) {
        List<T> rows = jdbc.query("SELECT * FROM " + tableName() + " WHERE id = ?", this::mapRow, id);
        return rows.isEmpty() ? null : rows.get(0);
    }

    @Override
    public List<T> findAll() {
        return jdbc.query("SELECT * FROM " + tableName(), this::mapRow);
    }

    @Override
    public T save(T entity) {
        Map<String, Object> cols = toMap(entity);
        Object rawId = cols.get("id");
        if (rawId == null) {
            rawId = UUID.randomUUID().toString();
            cols.put("id", rawId);
        }

        ID id = getId(entity);
        boolean exists = id != null && findById(id) != null;

        if (!exists) {
            String columns = String.join(", ", cols.keySet());
            String placeholders = cols.keySet().stream().map(k -> "?").collect(Collectors.joining(", "));
            jdbc.update("INSERT INTO " + tableName() + " (" + columns + ") VALUES (" + placeholders + ")",
                    cols.values().toArray());
            return findById(parseId(rawId));
        }

        String setClause = cols.keySet().stream()
                .filter(k -> !k.equals("id"))
                .map(k -> k + " = ?")
                .collect(Collectors.joining(", "));
        List<Object> params = cols.entrySet().stream()
                .filter(e -> !e.getKey().equals("id"))
                .map(Map.Entry::getValue)
                .collect(Collectors.toList());
        params.add(rawId);
        jdbc.update("UPDATE " + tableName() + " SET " + setClause + " WHERE id = ?", params.toArray());
        return findById(id);
    }

    @Override
    public void delete(ID id) {
        jdbc.update("DELETE FROM " + tableName() + " WHERE id = ?", id);
    }

    @Override
    public long count() {
        Long result = jdbc.queryForObject("SELECT COUNT(*) FROM " + tableName(), Long.class);
        return result != null ? result : 0L;
    }
}
