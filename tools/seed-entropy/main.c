// seed-entropy: credits entropy to the kernel CRNG.
//
// Modes:
//   seed-entropy              jitter-based self-seed (for fresh boot)
//   seed-entropy /path/file   read seed from file, credit it, delete file
//
// The file mode is used after snapshot restore: the host writes random
// bytes into the rootfs at /entropy-seed before resume, and the guest
// init script runs "seed-entropy /entropy-seed" to credit them.
#include <fcntl.h>
#include <linux/random.h>
#include <stdio.h>
#include <string.h>
#include <sys/ioctl.h>
#include <time.h>
#include <unistd.h>

static int credit_entropy(const unsigned char *data, int len) {
    struct {
        int entropy_count;
        int buf_size;
        unsigned char buf[512];
    } ent;

    int chunk = len < 512 ? len : 512;
    memcpy(ent.buf, data, chunk);
    ent.entropy_count = chunk * 8;
    ent.buf_size = chunk;

    int fd = open("/dev/urandom", O_WRONLY);
    if (fd < 0) return -1;
    int rc = ioctl(fd, RNDADDENTROPY, &ent);
    close(fd);
    return rc;
}

int main(int argc, char *argv[]) {
    if (argc > 1) {
        // File mode: read seed file, credit it, delete it
        int fd = open(argv[1], O_RDONLY);
        if (fd < 0) return 1;
        unsigned char buf[512];
        int n = read(fd, buf, sizeof(buf));
        close(fd);
        if (n <= 0) return 1;
        unlink(argv[1]);
        return credit_entropy(buf, n) < 0 ? 1 : 0;
    }

    // Jitter mode: gather entropy from high-res timer
    unsigned char buf[512];
    struct timespec ts;
    for (int i = 0; i < 512; i++) {
        clock_gettime(CLOCK_MONOTONIC, &ts);
        buf[i] = (unsigned char)(ts.tv_nsec ^ (ts.tv_nsec >> 8) ^ (ts.tv_nsec >> 16));
        for (volatile int j = 0; j < (ts.tv_nsec & 0x1f) + 1; j++) {}
    }
    return credit_entropy(buf, 512) < 0 ? 1 : 0;
}
