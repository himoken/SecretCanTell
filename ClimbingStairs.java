public class ClimbingStairs {
    public int climbStairs(int n) {
        if (n < 3) return n; // Special case
        int n_2 = 1; // f(n) = f(n-2) + f(n-1)
        int n_1 = 2;
        int ans = 0;
        for (int i = 3; i <= n; i++) {
            ans = n_2 + n_1;
            n_2 = n_1;
            n_1 = ans;        
        }
        return ans;
    }
}