package {{.GroupId}}.service;

import {{.GroupId}}.entity.User;
import {{.GroupId}}.repository.UserRepository;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

@Service
public class UserService {

    private final UserRepository users;

    public UserService(UserRepository users) {
        this.users = users;
    }

    public User create(String name, String email) {
        User u = new User(null, name, email, Instant.now());
        return users.save(u);
    }

    public User findById(UUID id) {
        return users.findById(id);
    }

    public List<User> findAll() {
        return users.findAll();
    }

    public User update(UUID id, String name, String email) {
        User u = new User(id, name, email, null);
        return users.save(u);
    }

    public void delete(UUID id) {
        users.delete(id);
    }
}
