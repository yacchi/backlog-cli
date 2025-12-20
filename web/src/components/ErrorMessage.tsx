interface ErrorMessageProps {
  message: string;
}

export default function ErrorMessage({ message }: ErrorMessageProps) {
  if (!message) return null;

  return (
    <div className="bg-red-50 text-red-700 p-4 rounded-lg mb-6 text-left">
      {message}
    </div>
  );
}
