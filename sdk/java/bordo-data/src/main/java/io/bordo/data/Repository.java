package io.bordo.data;

import java.util.List;

/**
 * Base repository interface for CRUD operations on a domain entity.
 *
 * @param <T>  entity type
 * @param <ID> identifier type
 */
public interface Repository<T, ID> {
    T findById(ID id);
    List<T> findAll();
    T save(T entity);
    void delete(ID id);
    long count();
}
