import { useEffect, useState } from "react";
import { onSocketStatus } from "../../lib/api";

const MESSAGE =
  "Docker socket unreachable at /var/run/docker.sock — is the daemon running, and is this user in the docker group? Run vessel doctor.";

/** SPEC §5.2: docker_unreachable gets a banner, never a generic toast. */
export function SocketBanner() {
  const [unreachable, setUnreachable] = useState(false);

  useEffect(() => onSocketStatus(setUnreachable), []);

  if (!unreachable) return null;

  return (
    <div role="alert" className="border-b border-fail bg-[#FDF8EC] px-4 py-2 text-[13px] text-fail">
      {MESSAGE}
    </div>
  );
}
