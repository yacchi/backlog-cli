import { useSearchParams } from "react-router-dom";
import Container from "../components/Container";
import ResultView, { type ResultType } from "../components/ResultView";

export default function AuthComplete() {
  const [searchParams] = useSearchParams();
  const type = (searchParams.get("type") as ResultType) || "success";
  const message = searchParams.get("message") || undefined;

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <Container>
        <ResultView type={type} message={message} />
      </Container>
    </main>
  );
}
