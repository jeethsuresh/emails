import net from "node:net";
import tls from "node:tls";

export interface DialOptions {
  host: string;
  port: number;
  tls: boolean;
}

interface Conn {
  socket: net.Socket;
  readBuf: Buffer[];
  waiters: Array<(chunk: Buffer | null) => void>;
  closed: boolean;
}

/**
 * TCP bridge used by Go WASM IMAP/SMTP (browser WASM has no raw sockets).
 */
export class NetBridge {
  private nextId = 1;
  private conns = new Map<number, Conn>();

  async dial(opts: DialOptions): Promise<number> {
    const id = this.nextId++;
    const socket = await new Promise<net.Socket>((resolve, reject) => {
      const onErr = (err: Error) => reject(err);
      if (opts.tls) {
        const s = tls.connect(
          { host: opts.host, port: opts.port, servername: opts.host },
          () => {
            s.off("error", onErr);
            resolve(s);
          },
        );
        s.once("error", onErr);
      } else {
        const s = net.connect({ host: opts.host, port: opts.port }, () => {
          s.off("error", onErr);
          resolve(s);
        });
        s.once("error", onErr);
      }
    });

    const conn: Conn = {
      socket,
      readBuf: [],
      waiters: [],
      closed: false,
    };

    socket.on("data", (chunk) => {
      if (conn.waiters.length > 0) {
        const w = conn.waiters.shift()!;
        w(chunk);
      } else {
        conn.readBuf.push(chunk);
      }
    });

    socket.on("close", () => {
      conn.closed = true;
      while (conn.waiters.length) {
        conn.waiters.shift()!(null);
      }
      this.conns.delete(id);
    });

    socket.on("error", () => {
      conn.closed = true;
      this.conns.delete(id);
    });

    this.conns.set(id, conn);
    return id;
  }

  write(id: number, data: Uint8Array): number {
    const conn = this.conns.get(id);
    if (!conn || conn.closed) throw new Error(`conn ${id} closed`);
    conn.socket.write(Buffer.from(data));
    return data.byteLength;
  }

  async read(id: number, max: number): Promise<Uint8Array> {
    const conn = this.conns.get(id);
    if (!conn) return new Uint8Array();

    const take = (buf: Buffer) => {
      if (buf.length <= max) return new Uint8Array(buf);
      const out = buf.subarray(0, max);
      conn.readBuf.unshift(buf.subarray(max));
      return new Uint8Array(out);
    };

    if (conn.readBuf.length > 0) {
      return take(conn.readBuf.shift()!);
    }
    if (conn.closed) return new Uint8Array();

    const chunk = await new Promise<Buffer | null>((resolve) => {
      conn.waiters.push(resolve);
    });
    if (!chunk) return new Uint8Array();
    return take(chunk);
  }

  close(id: number) {
    const conn = this.conns.get(id);
    if (!conn) return;
    conn.closed = true;
    conn.socket.destroy();
    this.conns.delete(id);
  }

  closeAll() {
    for (const id of [...this.conns.keys()]) this.close(id);
  }
}
