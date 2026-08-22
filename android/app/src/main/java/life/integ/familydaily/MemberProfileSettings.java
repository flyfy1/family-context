package life.integ.familydaily;

final class MemberProfileSettings {
    static final String MEMBER = "member";
    static final String ELDER = "elder";
    static final String CHILD = "child";
    private MemberProfileSettings() {}

    static String normalizeRole(String role) {
        if (ELDER.equals(role)) return ELDER;
        if (CHILD.equals(role)) return CHILD;
        return MEMBER;
    }

    static final class Profile {
        final String id;
        final String name;
        final String role;

        Profile(String id, String name, String role) {
            this.id = id;
            this.name = name;
            this.role = role;
        }
    }
}
