// seed-entropy: credits jitter-based entropy to the kernel CRNG.
// Run this before any daemon that calls getrandom().
#include <fcntl.h>
#include <linux/random.h>
#include <stdio.h>
#include <string.h>
#include <sys/ioctl.h>
#include <time.h>
#include <unistd.h>

int main(void) {
    struct {
        int entropy_count;
        int buf_size;
        unsigned char buf[512];
    } ent;

    // Gather jitter-based entropy from high-res timer
    struct timespec ts;
    unsigned char *p = ent.buf;
    for (int i = 0; i < 512; i++) {
        clock_gettime(CLOCK_MONOTONIC, &ts);
        p[i] = (unsigned char)(ts.tv_nsec ^ (ts.tv_nsec >> 8) ^ (ts.tv_nsec >> 16));
        // Tight loop jitter — each iteration takes slightly different time
        for (volatile int j = 0; j < (ts.tv_nsec & 0x1f) + 1; j++) {}
    }

    ent.entropy_count = 512 * 8; // bits
    ent.buf_size = 512;

    int fd = open("/dev/urandom", O_WRONLY);
    if (fd < 0) {
        perror("open /dev/urandom");
        return 1;
    }

    if (ioctl(fd, RNDADDENTROPY, &ent) < 0) {
        perror("RNDADDENTROPY");
        close(fd);
        return 1;
    }

    close(fd);
    return 0;
}
